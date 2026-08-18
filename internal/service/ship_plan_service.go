package service

import (
	"context"
	"fmt"
	"strings"

	"shippingcore/internal/dto"
	"shippingcore/internal/integrations/ordercore"
	"shippingcore/internal/model"

	"gorm.io/gorm"
)

func toShipPlanLineDTO(l model.ShipPlanLine) dto.ShipPlanLineDTO {
	return dto.ShipPlanLineDTO{
		ID:               l.ID,
		OrderCoreID:      l.OrderCoreID,
		OrderItemID:      l.OrderItemID,
		SplitOrderItemID: l.SplitOrderItemID,
		SkuName:          l.SkuName,
		Qty:              l.Qty,
		SortNo:           l.SortNo,
		Status:           l.Status,
	}
}

// GetShipPlan 查询订单发货计划行（默认含全部状态；status 可筛 pending/shipped）。
func (s *ShipmentService) GetShipPlan(orderCoreID uint64, status string) ([]dto.ShipPlanLineDTO, error) {
	if orderCoreID == 0 {
		return nil, ErrBadRequest
	}
	dbq := s.db().Model(&model.ShipPlanLine{}).
		Where("tenant_id = ? AND order_core_id = ?", s.tenantID, orderCoreID)
	if st := strings.TrimSpace(status); st != "" {
		dbq = dbq.Where("status = ?", st)
	}
	var rows []model.ShipPlanLine
	if err := dbq.Order("sort_no ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]dto.ShipPlanLineDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, toShipPlanLineDTO(r))
	}
	return out, nil
}

// PutShipPlan 覆盖保存 pending 计划行；shipped 行保留不动。lines 为空表示取消全部未发拆分。
// 保存成功后同步 OrderCore 拆分子行，并回写 split_order_item_id。
func (s *ShipmentService) PutShipPlan(ctx context.Context, token string, orderCoreID uint64, in *dto.PutShipPlanDTO) ([]dto.ShipPlanLineDTO, error) {
	if orderCoreID == 0 {
		return nil, ErrBadRequest
	}
	if in == nil {
		in = &dto.PutShipPlanDTO{}
	}

	prepared := make([]model.ShipPlanLine, 0, len(in.Lines))
	hasBound := false
	hasFree := false
	for i, line := range in.Lines {
		sku := strings.TrimSpace(line.SkuName)
		if sku == "" {
			return nil, fmt.Errorf("%w: 第 %d 行规格名称不能为空", ErrBadRequest, i+1)
		}
		qty := line.Qty
		if qty <= 0 {
			return nil, fmt.Errorf("%w: 第 %d 行数量须大于 0", ErrBadRequest, i+1)
		}
		if line.OrderItemID > 0 {
			hasBound = true
		} else {
			hasFree = true
		}
		sortNo := line.SortNo
		if sortNo == 0 {
			sortNo = i + 1
		}
		prepared = append(prepared, model.ShipPlanLine{
			TenantID:    s.tenantID,
			OrderCoreID: orderCoreID,
			OrderItemID: line.OrderItemID,
			SkuName:     sku,
			Qty:         qty,
			SortNo:      sortNo,
			Status:      model.ShipPlanStatusPending,
		})
	}
	if hasBound && hasFree {
		return nil, fmt.Errorf("%w: 整单拆分与按商品拆分不能混用，请统一后再保存", ErrBadRequest)
	}

	mode := ""
	if len(prepared) > 0 {
		if hasFree {
			mode = "full"
		} else {
			mode = "partial"
		}
	}

	err := s.db().Transaction(func(tx *gorm.DB) error {
		if err := tx.Where(
			"tenant_id = ? AND order_core_id = ? AND status = ?",
			s.tenantID, orderCoreID, model.ShipPlanStatusPending,
		).Delete(&model.ShipPlanLine{}).Error; err != nil {
			return err
		}
		if len(prepared) == 0 {
			return nil
		}
		return tx.Create(&prepared).Error
	})
	if err != nil {
		return nil, err
	}

	if err := s.syncSplitItemsToOrderCore(ctx, token, orderCoreID, mode, prepared); err != nil {
		return nil, err
	}
	return s.GetShipPlan(orderCoreID, "")
}

func (s *ShipmentService) syncSplitItemsToOrderCore(ctx context.Context, token string, orderCoreID uint64, mode string, pending []model.ShipPlanLine) error {
	if s.orderCore == nil {
		return fmt.Errorf("%w: 订单中心未配置，无法同步拆分行", ErrBadRequest)
	}
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("%w: 缺少鉴权，无法同步拆分行到订单中心", ErrBadRequest)
	}

	lines := make([]ordercore.SyncSplitItemLine, 0, len(pending))
	for _, p := range pending {
		if p.ID == 0 {
			continue
		}
		lines = append(lines, ordercore.SyncSplitItemLine{
			ParentOrderItemID: p.OrderItemID,
			SkuName:           p.SkuName,
			Qty:               p.Qty,
			ShipPlanLineID:    p.ID,
		})
	}
	// 取消拆分时也要通知 OrderCore 清理未发子行
	reqMode := mode
	if reqMode == "" && len(lines) == 0 {
		reqMode = "partial"
	}
	res, err := s.orderCore.SyncSplitItems(ctx, token, orderCoreID, ordercore.SyncSplitItemsRequest{
		Mode:  reqMode,
		Lines: lines,
	})
	if err != nil {
		return fmt.Errorf("同步订单中心拆分行失败: %w", err)
	}
	if res == nil || len(res.Lines) == 0 {
		return nil
	}
	for _, line := range res.Lines {
		if line.ShipPlanLineID == 0 || line.ID == 0 {
			continue
		}
		if err := s.db().Model(&model.ShipPlanLine{}).
			Where("tenant_id = ? AND id = ?", s.tenantID, line.ShipPlanLineID).
			Update("split_order_item_id", line.ID).Error; err != nil {
			return err
		}
	}
	return nil
}

// MarkShipPlanLinesShipped 将指定计划行标为已发。
func (s *ShipmentService) MarkShipPlanLinesShipped(ids []uint64) error {
	clean := make([]uint64, 0, len(ids))
	seen := map[uint64]struct{}{}
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		clean = append(clean, id)
	}
	if len(clean) == 0 {
		return nil
	}
	return s.db().Model(&model.ShipPlanLine{}).
		Where("tenant_id = ? AND id IN ? AND status = ?", s.tenantID, clean, model.ShipPlanStatusPending).
		Update("status", model.ShipPlanStatusShipped).Error
}

// ApplyShipPlanProgressFromGoods 按本次发货件数扣减计划行：发完标 shipped，未发完则减少 qty。
func (s *ShipmentService) ApplyShipPlanProgressFromGoods(goods []dto.OrderGoodsDTO) error {
	shippedByPlan := map[uint64]int{}
	for _, g := range goods {
		if g.PlanLineID == 0 {
			continue
		}
		n := g.Num
		if n <= 0 {
			n = 1
		}
		shippedByPlan[g.PlanLineID] += n
	}
	return s.applyShipPlanProgress(shippedByPlan)
}

// ApplyShipPlanProgressFromSplitLines 按拆分确认行扣减计划。
func (s *ShipmentService) ApplyShipPlanProgressFromSplitLines(lines []dto.SplitShipLineDTO) error {
	shippedByPlan := map[uint64]int{}
	for _, l := range lines {
		if l.PlanLineID == 0 {
			continue
		}
		n := l.Qty
		if n <= 0 {
			continue
		}
		shippedByPlan[l.PlanLineID] += n
	}
	return s.applyShipPlanProgress(shippedByPlan)
}

func (s *ShipmentService) applyShipPlanProgress(shippedByPlan map[uint64]int) error {
	if len(shippedByPlan) == 0 {
		return nil
	}
	ids := make([]uint64, 0, len(shippedByPlan))
	for id := range shippedByPlan {
		ids = append(ids, id)
	}
	var rows []model.ShipPlanLine
	if err := s.db().Where(
		"tenant_id = ? AND id IN ? AND status = ?",
		s.tenantID, ids, model.ShipPlanStatusPending,
	).Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		shipped := shippedByPlan[row.ID]
		if shipped <= 0 {
			continue
		}
		if shipped >= row.Qty {
			if err := s.db().Model(&row).Update("status", model.ShipPlanStatusShipped).Error; err != nil {
				return err
			}
			continue
		}
		left := row.Qty - shipped
		if err := s.db().Model(&row).Update("qty", left).Error; err != nil {
			return err
		}
	}
	return nil
}

// CountPendingShipPlanByOrders 批量统计各订单 pending 计划行数。
func (s *ShipmentService) CountPendingShipPlanByOrders(orderIDs []uint64) (map[uint64]int64, error) {
	out := map[uint64]int64{}
	if len(orderIDs) == 0 {
		return out, nil
	}
	type row struct {
		OrderCoreID uint64
		Cnt         int64
	}
	var rows []row
	err := s.db().Model(&model.ShipPlanLine{}).
		Select("order_core_id, COUNT(*) AS cnt").
		Where("tenant_id = ? AND order_core_id IN ? AND status = ?", s.tenantID, orderIDs, model.ShipPlanStatusPending).
		Group("order_core_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.OrderCoreID] = r.Cnt
	}
	return out, nil
}

func collectPlanLineIDsFromGoods(goods []dto.OrderGoodsDTO) []uint64 {
	ids := make([]uint64, 0)
	seen := map[uint64]struct{}{}
	for _, g := range goods {
		if g.PlanLineID == 0 {
			continue
		}
		if _, ok := seen[g.PlanLineID]; ok {
			continue
		}
		seen[g.PlanLineID] = struct{}{}
		ids = append(ids, g.PlanLineID)
	}
	return ids
}

func collectPlanLineIDsFromSplitLines(lines []dto.SplitShipLineDTO) []uint64 {
	ids := make([]uint64, 0)
	seen := map[uint64]struct{}{}
	for _, l := range lines {
		if l.PlanLineID == 0 {
			continue
		}
		if _, ok := seen[l.PlanLineID]; ok {
			continue
		}
		seen[l.PlanLineID] = struct{}{}
		ids = append(ids, l.PlanLineID)
	}
	return ids
}
