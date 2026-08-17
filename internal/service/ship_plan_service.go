package service

import (
	"fmt"
	"strings"

	"shippingcore/internal/dto"
	"shippingcore/internal/model"

	"gorm.io/gorm"
)

func toShipPlanLineDTO(l model.ShipPlanLine) dto.ShipPlanLineDTO {
	return dto.ShipPlanLineDTO{
		ID:          l.ID,
		OrderCoreID: l.OrderCoreID,
		OrderItemID: l.OrderItemID,
		SkuName:     l.SkuName,
		Qty:         l.Qty,
		SortNo:      l.SortNo,
		Status:      l.Status,
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
func (s *ShipmentService) PutShipPlan(orderCoreID uint64, in *dto.PutShipPlanDTO) ([]dto.ShipPlanLineDTO, error) {
	if orderCoreID == 0 {
		return nil, ErrBadRequest
	}
	if in == nil {
		in = &dto.PutShipPlanDTO{}
	}

	prepared := make([]model.ShipPlanLine, 0, len(in.Lines))
	for i, line := range in.Lines {
		sku := strings.TrimSpace(line.SkuName)
		if line.OrderItemID == 0 {
			return nil, fmt.Errorf("%w: 第 %d 行缺少原商品 ID", ErrBadRequest, i+1)
		}
		if sku == "" {
			return nil, fmt.Errorf("%w: 第 %d 行规格名称不能为空", ErrBadRequest, i+1)
		}
		qty := line.Qty
		if qty <= 0 {
			return nil, fmt.Errorf("%w: 第 %d 行数量须大于 0", ErrBadRequest, i+1)
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
	return s.GetShipPlan(orderCoreID, "")
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
