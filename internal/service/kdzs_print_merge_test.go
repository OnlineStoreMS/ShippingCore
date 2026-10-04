package service

import "testing"

func TestPrintMergeKeyAndOrders(t *testing.T) {
	a := map[string]any{
		"platform":         "DFHAND",
		"templateId":       "t1",
		"templateName":     "菜鸟面单",
		"kdzsAccountCode":  "acc1",
		"printerName":      "",
		"orderTimeFrom":    "2026-10-01 00:00:00",
		"orderTimeTo":      "2026-10-01 23:59:59",
		"autoConfirmShip":  true,
		"orderId":          float64(11),
		"orders": []any{
			map[string]any{"orderId": float64(11), "orderNo": "H1"},
		},
	}
	b := map[string]any{
		"platform":        "DFHAND",
		"templateId":      "t1",
		"templateName":    "菜鸟面单",
		"kdzsAccountCode": "acc1",
		"printerName":     "HP-1",
		"orderTimeFrom":   "2026-10-02 00:00:00",
		"orderTimeTo":     "2026-10-02 23:59:59",
		"orders": []any{
			map[string]any{"orderId": float64(12), "orderNo": "H2"},
			map[string]any{"orderId": float64(11), "orderNo": "H1"}, // dup
		},
	}
	ka, oka := printMergeKey(a)
	kb, okb := printMergeKey(b)
	if !oka || !okb || ka != kb {
		t.Fatalf("merge key mismatch: %q %q", ka, kb)
	}
	merged, n := mergePrintPayload(a, b)
	if n != 2 {
		t.Fatalf("want 2 orders got %d", n)
	}
	if _, ok := merged["autoConfirmShip"]; ok {
		t.Fatal("autoConfirmShip should be cleared for batch")
	}
	if payloadString(merged, "printerName") != "HP-1" {
		t.Fatalf("printer prefer non-empty, got %q", payloadString(merged, "printerName"))
	}
	if payloadString(merged, "orderTimeFrom") != "2026-10-01 00:00:00" {
		t.Fatalf("time from: %q", payloadString(merged, "orderTimeFrom"))
	}
	if payloadString(merged, "orderTimeTo") != "2026-10-02 23:59:59" {
		t.Fatalf("time to: %q", payloadString(merged, "orderTimeTo"))
	}
}
