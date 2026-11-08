package core

import "testing"

func TestModerate(t *testing.T) {
	if d, _ := Moderate("夜航", "凌晨的城市"); d != "allow" {
		t.Fatalf("expected allow, got %s", d)
	}
	if d, reason := Moderate("加微信领福利", ""); d != "reject" || reason == "" {
		t.Fatalf("expected reject with reason, got %s %q", d, reason)
	}
}

func TestValidateParams(t *testing.T) {
	cases := []struct {
		styles   []string
		duration int
		quality  string
		ok       bool
	}{
		{[]string{"电子"}, 30, "标准", true},
		{[]string{"电子", "Lo-fi"}, 60, "高清", true},
		{[]string{}, 30, "标准", false},
		{[]string{"a", "b", "c", "d"}, 30, "标准", false},
		{[]string{"电子"}, 20, "标准", false},
		{[]string{"电子"}, 30, "超清", false},
	}
	for _, c := range cases {
		err := validateParams(c.styles, c.duration, c.quality)
		if (err == nil) != c.ok {
			t.Fatalf("validateParams(%v,%d,%s) err=%v want ok=%v", c.styles, c.duration, c.quality, err, c.ok)
		}
	}
}
