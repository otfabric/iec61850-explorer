// SPDX-License-Identifier: MIT

package engine

import (
	"testing"
	"time"

	"github.com/otfabric/go-mms"
)

func TestParseValue(t *testing.T) {
	type tc struct {
		text string
		ts   *mms.TypeSpec
		want string // the text convertValue shows; "" means an error
	}
	f32 := &mms.TypeSpec{Type: mms.ValueTypeFloat, FormatWidth: 32}
	f64 := &mms.TypeSpec{Type: mms.ValueTypeFloat, FormatWidth: 64}
	cases := []tc{
		{"true", &mms.TypeSpec{Type: mms.ValueTypeBoolean}, "true"},
		{" 0 ", &mms.TypeSpec{Type: mms.ValueTypeBoolean}, "false"},
		{"yes", &mms.TypeSpec{Type: mms.ValueTypeBoolean}, ""},
		{"-128", &mms.TypeSpec{Type: mms.ValueTypeInteger, Size: 8}, "-128"},
		{"128", &mms.TypeSpec{Type: mms.ValueTypeInteger, Size: 8}, ""},
		{"9223372036854775807", &mms.TypeSpec{Type: mms.ValueTypeInteger}, "9223372036854775807"},
		{"1.5", &mms.TypeSpec{Type: mms.ValueTypeInteger, Size: 32}, ""},
		{"65535", &mms.TypeSpec{Type: mms.ValueTypeUnsigned, Size: 16}, "65535"},
		{"-1", &mms.TypeSpec{Type: mms.ValueTypeUnsigned, Size: 16}, ""},
		{"230.67", f32, "230.67"},
		{"1e39", f32, ""},
		{"NaN", f64, ""},
		{"0.1", f64, "0.1"},
		{"x", f64, ""},
		{"ok", &mms.TypeSpec{Type: mms.ValueTypeVisibleString, Size: -255}, "ok"},
		{"abc", &mms.TypeSpec{Type: mms.ValueTypeVisibleString, Size: 2}, ""},
		{"tab\there", &mms.TypeSpec{Type: mms.ValueTypeVisibleString}, ""},
		{"größe", &mms.TypeSpec{Type: mms.ValueTypeMmsString}, "größe"},
		{"0110", &mms.TypeSpec{Type: mms.ValueTypeBitString, Size: 4}, "0110"},
		{"01", &mms.TypeSpec{Type: mms.ValueTypeBitString, Size: 4}, ""},
		{"012", &mms.TypeSpec{Type: mms.ValueTypeBitString}, ""},
		{"", &mms.TypeSpec{Type: mms.ValueTypeBitString}, ""},
		{"0101", &mms.TypeSpec{Type: mms.ValueTypeBitString, Size: -2}, ""},
		{"de ad", &mms.TypeSpec{Type: mms.ValueTypeOctetString}, "dead"},
		{"dead", &mms.TypeSpec{Type: mms.ValueTypeOctetString, Size: 1}, ""},
		{"dead", &mms.TypeSpec{Type: mms.ValueTypeOctetString, Size: -1}, ""},
		{"zz", &mms.TypeSpec{Type: mms.ValueTypeOctetString}, ""},
		{"2026-11-09T12:00:00.5Z", &mms.TypeSpec{Type: mms.ValueTypeUTCTime}, "2026-11-09T12:00:00.500000Z"},
		{"yesterday", &mms.TypeSpec{Type: mms.ValueTypeUTCTime}, ""},
		{"1", &mms.TypeSpec{Type: mms.ValueTypeStructure}, ""},
		{"1", nil, ""},
	}
	for _, c := range cases {
		v, err := parseValue(c.text, c.ts)
		if c.want == "" {
			if err == nil {
				t.Errorf("parseValue(%q, %v) = %v, want an error", c.text, c.ts, v)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseValue(%q, %v): %v", c.text, c.ts, err)
			continue
		}
		if got := convertValue(v, c.ts, "").Text; got != c.want {
			t.Errorf("parseValue(%q) shows %q, want %q", c.text, got, c.want)
		}
	}
	if v, _ := parseValue("230.67", f32); v != nil {
		if f, _ := v.Float64(); f != float64(float32(230.67)) {
			t.Errorf("FLOAT32 sent as %v", f)
		}
	}
}

func TestSameValue(t *testing.T) {
	ts := &mms.TypeSpec{Type: mms.ValueTypeUTCTime}
	at := time.Date(2026, 11, 9, 12, 0, 0, 123_456_000, time.UTC)
	a := convertValue(mms.NewUTCTime(at), ts, "t")
	b := convertValue(mms.NewUTCTime(at.Add(200*time.Nanosecond)), ts, "t")
	c := convertValue(mms.NewUTCTime(at.Add(time.Millisecond)), ts, "t")
	if !sameValue(a, b) || sameValue(a, c) {
		t.Errorf("time comparison: %v %v %v", a.Text, b.Text, c.Text)
	}
	if sameValue(convertValue(mms.NewInteger(1), nil, ""), convertValue(mms.NewUnsigned(1), nil, "")) {
		t.Error("an INT equals an INTU")
	}
	if textOf(nil) != "" {
		t.Error("textOf(nil)")
	}
}
