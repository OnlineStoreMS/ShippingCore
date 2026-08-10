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

func TestBuildSFCargoDetailsUsesSpec(t *testing.T) {
	// 历史数据：GoodsName=商品名，SkuCode=规格名 → 下单须用规格
	sh := &model.Shipment{
		CargoName: "文件",
		Items: []model.ShipmentItem{
			{GoodsName: "山地车", SkuCode: "R7101-红-26寸", Quantity: 2},
			{GoodsName: "配件包", SkuCode: "配件-A", Quantity: 1},
		},
	}
	cargos := buildSFCargoDetails(sh)
	if len(cargos) != 2 || cargos[0].Name != "R7101-红-26寸" || cargos[0].Count != 2 {
		t.Fatalf("%#v", cargos)
	}
	if cargos[1].Name != "配件-A" {
		t.Fatalf("%#v", cargos)
	}
	if name := shipmentSFCargoName(sh); name != "R7101-红-26寸" {
		t.Fatalf("cargoName=%q", name)
	}
}

func TestResolveSFPrintTemplates(t *testing.T) {
	c := &model.CarrierAccount{
		PartnerID:    "XSZFMAB1WY1P",
		TemplateCode: "fm_76130_standard_custom_10058011961_1",
	}
	std, custom := resolveSFPrintTemplates(c)
	if custom != "fm_76130_standard_custom_10058011961_1" {
		t.Fatalf("custom=%s", custom)
	}
	if std != "fm_76130_standard_XSZFMAB1WY1P" {
		t.Fatalf("std=%s want partner standard", std)
	}

	c2 := &model.CarrierAccount{
		PartnerID:          "XSZFMAB1WY1P",
		TemplateCode:       "fm_76130_standard_XSZFMAB1WY1P",
		CustomTemplateCode: "fm_76130_standard_custom_10058011961_1",
	}
	std2, custom2 := resolveSFPrintTemplates(c2)
	if std2 != "fm_76130_standard_XSZFMAB1WY1P" || custom2 != "fm_76130_standard_custom_10058011961_1" {
		t.Fatalf("std=%s custom=%s", std2, custom2)
	}
}
