package sf

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeSignMode(t *testing.T) {
	cases := map[string]string{
		"":         SignModeSimple,
		"simple":   SignModeSimple,
		"STANDARD": SignModeStandard,
		"标准MD5":   SignModeStandard,
		"sm3":      SignModeSM3,
		"国密":       SignModeSM3,
		"unknown":  SignModeSimple,
	}
	for in, want := range cases {
		if got := NormalizeSignMode(in); got != want {
			t.Fatalf("NormalizeSignMode(%q)=%q want %q", in, got, want)
		}
	}
}

func TestComputeMsgDigestModesDiffer(t *testing.T) {
	msgData := `{"orderId":"TEST001"}`
	timestamp := "1700000000000"
	checkword := "secret"

	simple := ComputeMsgDigest(msgData, timestamp, checkword, SignModeSimple)
	standard := ComputeMsgDigest(msgData, timestamp, checkword, SignModeStandard)
	sm3dig := ComputeMsgDigest(msgData, timestamp, checkword, SignModeSM3)

	if simple == "" || standard == "" || sm3dig == "" {
		t.Fatal("expected non-empty digests")
	}
	if simple == standard || simple == sm3dig || standard == sm3dig {
		t.Fatalf("digests should differ: simple=%s standard=%s sm3=%s", simple, standard, sm3dig)
	}
	if simple != ComputeMsgDigestSimple(msgData, timestamp, checkword) {
		t.Fatal("simple mismatch")
	}
	if standard != ComputeMsgDigestStandard(msgData, timestamp, checkword) {
		t.Fatal("standard mismatch")
	}
	if sm3dig != ComputeMsgDigestSM3(msgData, timestamp, checkword) {
		t.Fatal("sm3 mismatch")
	}
}

func TestComputeMsgDigestOfficialSampleStandard(t *testing.T) {
	// 丰桥文档样例（标准MD5）：URLEncode(msgData+timestamp+checkWord) → MD5 → Base64
	msgData := `{"language":"zh-CN","orderId":"QIAO-20200618-004"}`
	timestamp := "12312334453453"
	checkword := "fjcg5PGKaNpPSHFAZ4QsCOkV71R3zVci"
	want := "IIKJtuLVzoFTu4kHI8M8vA=="
	got := ComputeMsgDigestStandard(msgData, timestamp, checkword)
	if got != want {
		t.Fatalf("standard digest mismatch:\n got  %s\n want %s", got, want)
	}
}

func TestParseCloudPrintParsedEnvelope(t *testing.T) {
	raw := `{"success":true,"requestId":"r1","obj":{"clientCode":"X","fileType":"json","templateCode":"fm_76130_standard_X","files":[{"waybillNo":"SF1","contents":[{"area":"master","items":[],"page":{"width":76,"height":130}}]}]}}`
	var envelope struct {
		Success   bool            `json:"success"`
		RequestID string          `json:"requestId"`
		Obj       json.RawMessage `json:"obj"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.Success || len(envelope.Obj) == 0 {
		t.Fatal("bad envelope")
	}
	var objMeta struct {
		Files json.RawMessage `json:"files"`
	}
	if err := json.Unmarshal(envelope.Obj, &objMeta); err != nil {
		t.Fatal(err)
	}
	if len(objMeta.Files) < 10 {
		t.Fatalf("files too short: %s", objMeta.Files)
	}
	if !strings.Contains(string(objMeta.Files), "contents") {
		t.Fatal("expected contents preserved in files json")
	}
}

func TestExtractPrintFileFromObj(t *testing.T) {
	// 丰桥同步云打印成功结构：files 在顶层 obj 下
	raw := `{"obj":{"clientCode":"XSZFMAB1WY1P","fileType":"pdf","files":[{"token":"AUTH_tk","url":"https://example.com/a.pdf","waybillNo":"SF1"}],"templateCode":"fm_76130_standard_X"},"requestId":"r1","success":true}`
	var result printMsgData
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatal("expected success")
	}
	u, tok := extractPrintFile(result)
	if u != "https://example.com/a.pdf" {
		t.Fatalf("url=%q", u)
	}
	if tok != "AUTH_tk" {
		t.Fatalf("token=%q", tok)
	}
}

func TestBuildPrintDocumentRemark(t *testing.T) {
	doc := buildPrintDocument("SF1", &PrintDocOptions{Remark: "托寄物:文件"})
	if doc["masterWaybillNo"] != "SF1" || doc["remark"] != "托寄物:文件" {
		t.Fatalf("%#v", doc)
	}
}

func TestOAuthURLByEnv(t *testing.T) {
	sbox := NewClient("P", "S", "sandbox")
	if sbox.OAuthURL() != SandboxOAuthURL {
		t.Fatalf("sandbox oauth url: %s", sbox.OAuthURL())
	}
	prod := NewClient("P", "S", "prod")
	if prod.OAuthURL() != ProdOAuthURL {
		t.Fatalf("prod oauth url: %s", prod.OAuthURL())
	}
}

func TestParseOAuthAccessTokenResponse(t *testing.T) {
	body := []byte(`{"apiResultCode":"A1000","apiErrorMsg":"成功","accessToken":"tok-abc","expiresIn":7199}`)
	token, exp, err := parseOAuthAccessTokenResponse(body)
	if err != nil {
		t.Fatal(err)
	}
	if token != "tok-abc" || exp != 7199 {
		t.Fatalf("token=%q exp=%d", token, exp)
	}

	bad := []byte(`{"apiResultCode":"A1011","apiErrorMsg":"参数错误"}`)
	if _, _, err := parseOAuthAccessTokenResponse(bad); err == nil {
		t.Fatal("expected error for A1011")
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
