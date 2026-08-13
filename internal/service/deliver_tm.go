package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"shippingcore/internal/carrier/sf"
	"shippingcore/internal/dto"
)

// QueryDeliverTm 按寄/收件地址查询物流产品时效与预估运费。
func (s *CarrierService) QueryDeliverTm(ctx context.Context, in dto.QueryDeliverTmRequest) (*dto.QueryDeliverTmResult, error) {
	if in.CarrierAccountID == 0 {
		return nil, fmt.Errorf("%w: 请选择物流账号", ErrBadRequest)
	}
	carrier, err := s.GetRaw(in.CarrierAccountID)
	if err != nil {
		return nil, err
	}
	if !carrier.Enabled {
		return nil, fmt.Errorf("%w: 物流账号已停用", ErrBadRequest)
	}

	srcP, srcC, srcD, srcA := strings.TrimSpace(in.SrcProvince), strings.TrimSpace(in.SrcCity), strings.TrimSpace(in.SrcCounty), strings.TrimSpace(in.SrcAddress)
	dstP, dstC, dstD, dstA := strings.TrimSpace(in.DestProvince), strings.TrimSpace(in.DestCity), strings.TrimSpace(in.DestCounty), strings.TrimSpace(in.DestAddress)
	if srcP == "" || srcC == "" || srcA == "" {
		return nil, fmt.Errorf("%w: 请填写寄件地址", ErrBadRequest)
	}
	if dstP == "" || dstC == "" || dstA == "" {
		return nil, fmt.Errorf("%w: 请填写收件地址", ErrBadRequest)
	}

	weight := in.WeightKG
	if weight <= 0 {
		weight = 1
	}

	monthly := ""
	if in.UseMonthly && strings.TrimSpace(carrier.CustID) != "" {
		monthly = carrier.CustID
	}

	client := sf.NewClientWithSignMode(carrier.PartnerID, carrier.Checkword, carrier.Env, carrier.SignMode)
	// 不传 BusinessType，以便一次返回多产品时效/价格（特快/标快等）
	win, err := client.QueryDeliverTm(ctx, sf.QueryDeliverTmRequest{
		SrcProvince:   srcP,
		SrcCity:       srcC,
		SrcDistrict:   srcD,
		SrcAddress:    srcA,
		DestProvince:  dstP,
		DestCity:      dstC,
		DestDistrict:  dstD,
		DestAddress:   dstA,
		WeightKG:      weight,
		ConsignedTime: strings.TrimSpace(in.ConsignedTime),
		MonthlyCard:   monthly,
	})
	if err != nil {
		return nil, err
	}

	now := time.Now()
	products := make([]dto.DeliverProductOption, 0, len(win.Items)+2)
	seen := map[string]bool{}
	var minFee float64
	var minFeeSet bool
	var earliest time.Time
	var earliestSet bool
	earliestCodes := map[string]bool{}

	for _, it := range win.Items {
		code := strings.TrimSpace(it.BusinessType)
		if code == "" {
			continue
		}
		// 打单页只展示可下单的特快/标快；丰桥还会回「便利封/袋」等包装产品（如 113），不纳入选型。
		if !isCoreExpressType(code) {
			continue
		}
		endAt, timeLabel := formatDeliverTimeLabel(now, it.DeliverTime)
		name := strings.TrimSpace(it.BusinessTypeDesc)
		if name == "" {
			name = defaultExpressName(code)
		}
		products = append(products, dto.DeliverProductOption{
			Value:        code,
			Name:         name,
			Fee:          it.Fee,
			DeliverTime:  it.DeliverTime,
			DeliverLabel: timeLabel,
			Hint:         "预估仅供参考，以实际计费为准",
		})
		seen[code] = true
		if it.Fee > 0 && (!minFeeSet || it.Fee < minFee) {
			minFee, minFeeSet = it.Fee, true
		}
		if !endAt.IsZero() {
			if !earliestSet || endAt.Before(earliest) {
				earliest, earliestSet = endAt, true
				earliestCodes = map[string]bool{code: true}
			} else if endAt.Equal(earliest) {
				earliestCodes[code] = true
			}
		}
	}

	// 补齐常用产品（接口未返回时仍可选手动下单）
	for _, def := range []struct{ code, name, tag, hint string }{
		{"1", "顺丰特快", "时效最优", "时效更快，适合急件"},
		{"2", "顺丰标快", "经济实惠", "常规时效，性价比高"},
	} {
		if seen[def.code] {
			continue
		}
		products = append(products, dto.DeliverProductOption{
			Value: def.code,
			Name:  def.name,
			Tag:   def.tag,
			Hint:  def.hint,
		})
	}

	for i := range products {
		p := &products[i]
		if minFeeSet && p.Fee > 0 && p.Fee == minFee {
			p.Tag = "价格最优"
			continue
		}
		if earliestCodes[p.Value] {
			p.Tag = "时效最优"
			continue
		}
		if p.Tag == "" {
			if p.Value == "1" {
				p.Tag = "时效最优"
			} else if p.Value == "2" && !minFeeSet {
				p.Tag = "经济实惠"
			}
		}
	}

	return &dto.QueryDeliverTmResult{Products: products}, nil
}

func isCoreExpressType(code string) bool {
	switch strings.TrimSpace(code) {
	case "1", "2":
		return true
	default:
		return false
	}
}

func defaultExpressName(code string) string {
	switch code {
	case "1":
		return "顺丰特快"
	case "2":
		return "顺丰标快"
	default:
		return "物流产品 " + code
	}
}

// formatDeliverTimeLabel 将 "YYYY-MM-DD HH:mm:ss[,YYYY-MM-DD HH:mm:ss]" 转为「预计 明天 15:00 前送达」。
func formatDeliverTimeLabel(now time.Time, raw string) (endAt time.Time, label string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, ""
	}
	parts := strings.Split(raw, ",")
	pick := strings.TrimSpace(parts[len(parts)-1])
	if pick == "" {
		pick = strings.TrimSpace(parts[0])
	}
	t, err := time.ParseInLocation("2006-01-02 15:04:05", pick, now.Location())
	if err != nil {
		t, err = time.ParseInLocation("2006-01-02 15:04", pick, now.Location())
	}
	if err != nil {
		return time.Time{}, "预计 " + pick + " 前送达"
	}
	dayLabel := relativeDayLabel(now, t)
	return t, fmt.Sprintf("预计 %s %s 前送达", dayLabel, t.Format("15:04"))
}

func relativeDayLabel(now, t time.Time) string {
	ny, nm, nd := now.Date()
	ty, tm, td := t.Date()
	n0 := time.Date(ny, nm, nd, 0, 0, 0, 0, now.Location())
	t0 := time.Date(ty, tm, td, 0, 0, 0, 0, now.Location())
	diff := int(t0.Sub(n0).Hours() / 24)
	switch diff {
	case 0:
		return "今天"
	case 1:
		return "明天"
	case 2:
		return "后天"
	default:
		if ty == ny {
			return fmt.Sprintf("%d月%d日", int(tm), td)
		}
		return t.Format("01-02")
	}
}
