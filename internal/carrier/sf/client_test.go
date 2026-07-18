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
