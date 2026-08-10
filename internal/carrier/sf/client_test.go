package sf

import "testing"

func TestComputeMsgDigest(t *testing.T) {
	msgData := `{"orderId":"TEST001"}`
	timestamp := "1700000000000"
	checkword := "secret"

	got := ComputeMsgDigest(msgData, timestamp, checkword)
	if got == "" {
		t.Fatal("expected non-empty digest")
	}

	again := ComputeMsgDigest(msgData, timestamp, checkword)
	if got != again {
		t.Fatalf("digest not stable: %q vs %q", got, again)
	}

	changed := ComputeMsgDigest(msgData, timestamp, checkword+"x")
	if got == changed {
		t.Fatal("expected digest to change when checkword changes")
	}
}

func TestNormalizeCargoDetails(t *testing.T) {
	got := normalizeCargoDetails([]CargoDetail{
		{Name: "A", Count: 2},
		{Name: "  ", Count: 1},
		{Name: "B", Count: 0},
	}, "fallback")
	if len(got) != 2 {
		t.Fatalf("want 2 cargos, got %#v", got)
	}
	if got[0].Name != "A" || got[0].Count != 2 {
		t.Fatalf("first cargo: %#v", got[0])
	}
	if got[1].Name != "B" || got[1].Count != 1 {
		t.Fatalf("second cargo count default: %#v", got[1])
	}

	fallback := normalizeCargoDetails(nil, "")
	if len(fallback) != 1 || fallback[0].Name != "商品" || fallback[0].Count != 1 {
		t.Fatalf("fallback cargo: %#v", fallback)
	}
}
