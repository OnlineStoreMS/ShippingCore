package service

import (
	"testing"

	"shippingcore/internal/dto"
	"shippingcore/internal/model"
)

func TestOrderGoodsShipNamePrefersSpec(t *testing.T) {
	got := orderGoodsShipName(dto.OrderGoodsDTO{Title: "山地车", SkuName: "R7101-红-26寸"})
	if got != "R7101-红-26寸" {
		t.Fatalf("got %q", got)
	}
	got = orderGoodsShipName(dto.OrderGoodsDTO{Title: "山地车", SkuName: ""})
	if got != "山地车" {
		t.Fatalf("fallback got %q", got)
	}
}

func TestBuildSFCargoDetailsJoinsAllSpecs(t *testing.T) {
	// 标准模板托寄物：规格*数量 逗号拼接，末尾 * 总数量
	sh := &model.Shipment{
		CargoName:  "文件",
		CargoCount: 3,
		Items: []model.ShipmentItem{
			{GoodsName: "山地车", SkuCode: "R7101-红-26寸", Quantity: 2},
			{GoodsName: "配件包", SkuCode: "配件-A", Quantity: 1},
		},
	}
	cargos := buildSFCargoDetails(sh)
	want := "R7101-红-26寸*2, 配件-A*1 * 3"
	if len(cargos) != 1 || cargos[0].Name != want || cargos[0].Count != 3 {
		t.Fatalf("%#v", cargos)
	}
	if name := shipmentSFCargoName(sh); name != want {
		t.Fatalf("cargoName=%q", name)
	}
}

func TestResolveSFPrintTemplatesStandardOnly(t *testing.T) {
	c := &model.CarrierAccount{
		PartnerID:          "XSZFMAB1WY1P",
		TemplateCode:       "fm_76130_standard_XSZFMAB1WY1P",
		CustomTemplateCode: "fm_76130_standard_custom_10058011961_2",
	}
	std, custom := resolveSFPrintTemplates(c)
	if std != "fm_76130_standard_XSZFMAB1WY1P" {
		t.Fatalf("std=%s", std)
	}
	if custom != "" {
		t.Fatalf("custom should be ignored, got %s", custom)
	}

	c2 := &model.CarrierAccount{
		PartnerID:    "XSZFMAB1WY1P",
		TemplateCode: "fm_76130_standard_custom_10058011961_1",
	}
	std2, custom2 := resolveSFPrintTemplates(c2)
	if custom2 != "" {
		t.Fatalf("custom=%s", custom2)
	}
	if std2 != "fm_76130_standard_XSZFMAB1WY1P" {
		t.Fatalf("std=%s want partner standard", std2)
	}
}
