package payments

import (
	"net/url"
	"testing"
)

func TestEPaySignAndVerify(t *testing.T) {
	values := url.Values{}
	values.Set("pid", "1001")
	values.Set("type", "alipay")
	values.Set("out_trade_no", "IM202606230001")
	values.Set("money", "10.00")
	values.Set("name", "ImageMirror credits")
	values.Set("sign_type", "MD5")
	values.Set("sign", sign(values, "secret"))

	if !verify(values, "secret") {
		t.Fatal("expected valid signature")
	}
	values.Set("money", "1.00")
	if verify(values, "secret") {
		t.Fatal("expected tampered payload to fail verification")
	}
}

func TestParseMoneyCents(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want int64
	}{
		{name: "yuan", raw: "10", want: 1000},
		{name: "one decimal", raw: "10.5", want: 1050},
		{name: "two decimals", raw: "10.05", want: 1005},
		{name: "extra zero decimals", raw: "10.0500", want: 1005},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMoneyCents(tt.raw)
			if err != nil {
				t.Fatalf("parseMoneyCents() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("parseMoneyCents() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestParseMoneyCentsRejectsInvalidValues(t *testing.T) {
	for _, raw := range []string{"", "abc", "10.001", "-1", "1.2.3"} {
		t.Run(raw, func(t *testing.T) {
			if _, err := parseMoneyCents(raw); err == nil {
				t.Fatal("expected invalid money to fail")
			}
		})
	}
}

func TestNormalizePayType(t *testing.T) {
	got, err := normalizePayType("")
	if err != nil {
		t.Fatalf("normalizePayType() error = %v", err)
	}
	if got != "alipay" {
		t.Fatalf("normalizePayType() = %q, want alipay", got)
	}
	if _, err := normalizePayType("card"); err == nil {
		t.Fatal("expected unsupported payment type to fail")
	}
}
