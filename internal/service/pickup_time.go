package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"shippingcore/internal/carrier/sf"
	"shippingcore/internal/dto"
)

// CheckPickupTime 按寄件地址查询顺丰可揽收时间窗，并生成今明后小时段（含「1小时内」）。
func (s *CarrierService) CheckPickupTime(ctx context.Context, in dto.CheckPickupTimeRequest) (*dto.CheckPickupTimeResult, error) {
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

	fullAddr := joinShipperAddress(in.Province, in.City, in.County, in.Address)
	if fullAddr == "" {
		return nil, fmt.Errorf("%w: 请填写寄件地址", ErrBadRequest)
	}
	cityCode := strings.TrimSpace(in.CityCode)
	if cityCode == "" {
		cityCode = resolveSFCityCode(in.Province, in.City)
	}

	client := sf.NewClientWithSignMode(carrier.PartnerID, carrier.Checkword, carrier.Env, carrier.SignMode)
	win, err := client.CheckPickupTime(ctx, sf.CheckPickupTimeRequest{
		Address:  fullAddr,
		CityCode: cityCode,
		SendTime: time.Now().Format("2006-01-02 15:04:05"),
	})
	if err != nil {
		return nil, err
	}

	startTm := strings.TrimSpace(win.StartTm)
	endTm := strings.TrimSpace(win.EndTm)
	if startTm == "" {
		startTm = "0800"
	}
	if endTm == "" {
		endTm = "2000"
	}

	out := &dto.CheckPickupTimeResult{
		StartTm:         startTm,
		EndTm:           endTm,
		Status:          win.Status,
		ExceptionReason: win.ExceptionReason,
		CityCode:        cityCode,
		Address:         fullAddr,
		Options:         buildPickupAppointOptions(time.Now(), startTm, endTm),
	}
	return out, nil
}

func joinShipperAddress(province, city, county, address string) string {
	parts := make([]string, 0, 4)
	for _, p := range []string{province, city, county, address} {
		p = strings.TrimSpace(p)
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, "")
}

// resolveSFCityCode 常见城市 → 顺丰城市码（不足时仍可凭地址查询，但成功率较低）。
func resolveSFCityCode(province, city string) string {
	key := strings.TrimSpace(city)
	if key == "" {
		key = strings.TrimSpace(province)
	}
	key = strings.ReplaceAll(key, "市", "")
	key = strings.ReplaceAll(key, "地区", "")
	key = strings.ReplaceAll(key, "自治州", "")
	m := map[string]string{
		"深圳": "755", "广州": "020", "东莞": "769", "佛山": "757", "珠海": "756",
		"中山": "760", "惠州": "752", "汕头": "754", "湛江": "759",
		"杭州": "571", "宁波": "574", "温州": "577", "金华": "579", "嘉兴": "573", "绍兴": "575",
		"上海": "021", "南京": "025", "苏州": "512", "无锡": "510", "常州": "519", "南通": "513", "徐州": "516",
		"北京": "010", "天津": "022", "重庆": "023",
		"成都": "028", "武汉": "027", "长沙": "731", "郑州": "371", "西安": "029",
		"青岛": "532", "济南": "531", "大连": "411", "沈阳": "024", "哈尔滨": "451", "长春": "431",
		"福州": "591", "厦门": "592", "泉州": "595",
		"合肥": "551", "南昌": "791", "昆明": "871", "贵阳": "851", "南宁": "771", "海口": "898",
		"石家庄": "311", "太原": "351", "呼和浩特": "471", "兰州": "931", "乌鲁木齐": "991",
	}
	if code, ok := m[key]; ok {
		return code
	}
	return ""
}

func parseHHMM(v string) (int, bool) {
	v = strings.TrimSpace(v)
	v = strings.ReplaceAll(v, ":", "")
	if len(v) < 3 || len(v) > 4 {
		return 0, false
	}
	if len(v) == 3 {
		v = "0" + v
	}
	h := int(v[0]-'0')*10 + int(v[1]-'0')
	m := int(v[2]-'0')*10 + int(v[3]-'0')
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

func pad2(n int) string {
	if n < 10 {
		return fmt.Sprintf("0%d", n)
	}
	return fmt.Sprintf("%d", n)
}

func formatLocalDT(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}

// buildPickupAppointOptions 按官方时间窗 + 当前时间生成今明后小时段。
// 例：现在 15:34 → 今天最早小时段为 16:00-17:00；另提供「1小时内」（sendStartTm≈now+15m，对齐企服）。
func buildPickupAppointOptions(now time.Time, startTm, endTm string) []dto.PickupAppointDayOption {
	startMin, ok1 := parseHHMM(startTm)
	endMin, ok2 := parseHHMM(endTm)
	if !ok1 {
		startMin = 8 * 60
	}
	if !ok2 {
		endMin = 20 * 60
	}
	if endMin <= startMin {
		endMin = startMin + 60
	}

	dayLabels := []string{"今天", "明天", "后天"}
	out := make([]dto.PickupAppointDayOption, 0, 3)
	for dayOffset := 0; dayOffset <= 2; dayOffset++ {
		children := make([]dto.PickupAppointSlotOption, 0, 20)
		dayBase := time.Date(now.Year(), now.Month(), now.Day()+dayOffset, 0, 0, 0, 0, now.Location())

		if dayOffset == 0 {
			withinAt := now.Add(15 * time.Minute)
			withinMin := withinAt.Hour()*60 + withinAt.Minute()
			if withinMin >= startMin && withinMin < endMin {
				children = append(children, dto.PickupAppointSlotOption{
					Value:       fmt.Sprintf("%d|within1h", dayOffset),
					Text:        "1小时内",
					SlotKey:     "within1h",
					SendStartTm: formatLocalDT(withinAt),
				})
			}
		}

		// 今天：下一整点起（15:34 → 16:00）
		firstHourStart := startMin
		if dayOffset == 0 {
			nextHour := (now.Hour() + 1) * 60
			if now.Minute() == 0 && now.Second() == 0 && now.Nanosecond() == 0 {
				nextHour = now.Hour() * 60
			}
			if nextHour > firstHourStart {
				firstHourStart = nextHour
			}
		}
		// 对齐到整点小时
		if rem := firstHourStart % 60; rem != 0 {
			firstHourStart += 60 - rem
		}

		for slotStart := firstHourStart; slotStart < endMin; slotStart += 60 {
			h := slotStart / 60
			if h > 23 {
				break
			}
			slotEnd := slotStart + 60
			endH, endM := slotEnd/60, slotEnd%60
			if endH > 23 {
				endH, endM = 23, 59
			}
			sendAt := time.Date(dayBase.Year(), dayBase.Month(), dayBase.Day(), h, 0, 0, 0, now.Location())
			slotKey := fmt.Sprintf("%s:00", pad2(h))
			children = append(children, dto.PickupAppointSlotOption{
				Value:       fmt.Sprintf("%d|%s", dayOffset, slotKey),
				Text:        fmt.Sprintf("%s:00-%s:%s", pad2(h), pad2(endH), pad2(endM)),
				SlotKey:     slotKey,
				SendStartTm: formatLocalDT(sendAt),
			})
		}

		if len(children) == 0 {
			continue
		}
		out = append(out, dto.PickupAppointDayOption{
			Value:    dayOffset,
			Text:     dayLabels[dayOffset],
			Children: children,
		})
	}
	return out
}
