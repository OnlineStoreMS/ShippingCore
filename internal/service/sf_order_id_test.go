package service

import (
	"testing"

	"shippingcore/internal/model"
)

func TestFormatSFCustomerOrderID(t *testing.T) {
	cases := []struct {
		base string
		seq  int
		want string
	}{
		{"OC202608110031", 1, "OC202608110031"},
		{"OC202608110031", 2, "OC202608110031-2"},
		{"OC202608110031", 3, "OC202608110031-3"},
		{"", 2, ""},
	}
	for _, c := range cases {
		if got := formatSFCustomerOrderID(c.base, c.seq); got != c.want {
			t.Fatalf("base=%q seq=%d got=%q want=%q", c.base, c.seq, got, c.want)
		}
	}
}

func TestSFCustomerOrderBase(t *testing.T) {
	s := &model.Shipment{OrderNo: "OC202608110031", SourceTid: "other"}
	if got := sfCustomerOrderBase(s); got != "OC202608110031" {
		t.Fatalf("got=%s", got)
	}
}
