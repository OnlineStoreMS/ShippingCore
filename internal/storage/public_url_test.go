package storage

import "testing"

func TestPublicURLResolverRewritesHost(t *testing.T) {
	r := &PublicURLResolver{
		baseURL:   "https://osms.zfcycle.com/minio/shippingcore",
		keyPrefix: "attachments",
	}
	in := "http://192.168.3.41:9100/shippingcore/attachments/shipment-labels/20260814014611_71295273.pdf"
	want := "https://osms.zfcycle.com/minio/shippingcore/attachments/shipment-labels/20260814014611_71295273.pdf"
	if got := r.Resolve(in); got != want {
		t.Fatalf("Resolve() = %q, want %q", got, want)
	}
	// already on new base
	if got := r.Resolve(want); got != want {
		t.Fatalf("idempotent Resolve() = %q", got)
	}
	// non-http left alone
	if got := r.Resolve("sf-plugin://SF123"); got != "sf-plugin://SF123" {
		t.Fatalf("non-http = %q", got)
	}
}
