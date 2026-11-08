// SPDX-License-Identifier: MIT

package engine

import (
	"math"
	"testing"
	"time"

	iec61850 "github.com/otfabric/go-iec61850"
	"github.com/otfabric/go-mms"

	"github.com/otfabric/iec61850-explorer/internal/model"
)

func TestConvertScalars(t *testing.T) {
	f32 := float64(float32(230.67))
	for name, tc := range map[string]struct {
		v    *mms.Value
		ts   *mms.TypeSpec
		typ  string
		text string
		json any
	}{
		"bool":            {mms.NewBoolean(true), nil, model.TypeBoolean, "true", true},
		"int":             {mms.NewInteger(-7), nil, model.TypeInteger, "-7", int64(-7)},
		"big int":         {mms.NewInteger(1 << 60), nil, model.TypeInteger, "1152921504606846976", "1152921504606846976"},
		"unsigned":        {mms.NewUnsigned(9), nil, model.TypeUnsigned, "9", uint64(9)},
		"float32 typed":   {mms.NewFloat(f32), &mms.TypeSpec{Type: mms.ValueTypeFloat, FormatWidth: 32}, model.TypeFloat, "230.67", f32},
		"float32 width24": {mms.NewFloat(f32), &mms.TypeSpec{Type: mms.ValueTypeFloat, FormatWidth: 24}, model.TypeFloat, "230.67", f32},
		"float32 untyped": {mms.NewFloat(f32), nil, model.TypeFloat, "230.67", f32},
		"float64":         {mms.NewFloat(0.1), &mms.TypeSpec{Type: mms.ValueTypeFloat, FormatWidth: 64}, model.TypeFloat, "0.1", 0.1},
		"octets":          {mms.NewOctetString([]byte{0xde, 0xad}), nil, model.TypeOctetString, "dead", "dead"},
		"visible":         {mms.NewVisibleString("LD0"), nil, model.TypeVisible, "LD0", "LD0"},
		"bits":            {mms.NewBitStringWithLength([]byte{0x40}, 2), nil, model.TypeBitString, "01", "01"},
		"access error":    {mms.NewDataAccessError(mms.DataAccessErrorObjectNonExistent), nil, model.TypeAccessError, mms.DataAccessErrorObjectNonExistent.String(), nil},
	} {
		got := convertValue(tc.v, tc.ts, "")
		if got.Type != tc.typ || got.Text != tc.text || got.JSON != tc.json {
			t.Errorf("%s: %+v (json %#v), want %s %q %#v", name, got, got.JSON, tc.typ, tc.text, tc.json)
		}
	}
	if got := convertValue(mms.NewFloat(math.NaN()), nil, ""); got.JSON != nil || got.Text != "NaN" {
		t.Errorf("NaN = %+v", got)
	}
	if got := convertValue(nil, nil, "x"); got.Type != model.TypeOther {
		t.Errorf("nil = %+v", got)
	}
}

func TestConvertQualityAndTime(t *testing.T) {
	q := convertValue(iec61850.EncodeQuality(iec61850.Quality(iec61850.ValidityInvalid)|iec61850.QualityFailure|iec61850.QualityTest), nil, "q")
	if q.Quality == nil || q.Quality.Validity != "invalid" || q.Text != "invalid (failure, test)" {
		t.Errorf("quality = %+v", q)
	}
	// A 13-bit string named otherwise is not a quality.
	if other := convertValue(iec61850.EncodeQuality(0), nil, "x"); other.Quality != nil {
		t.Errorf("x = %+v", other)
	}
	at := time.Date(2026, 11, 8, 1, 2, 3, 0, time.UTC)
	ts := convertValue(iec61850.EncodeTimestamp(iec61850.Timestamp{Time: at, Quality: iec61850.TimeQuality{ClockNotSynchronized: true}}), nil, "t")
	if ts.Time == nil || !ts.Time.Time.Equal(at) || !ts.Time.ClockNotSynchronized || ts.Text != "2026-11-08T01:02:03.000000Z" {
		t.Errorf("time = %+v", ts)
	}
}

func TestConvertStructureNames(t *testing.T) {
	ts := &mms.TypeSpec{Type: mms.ValueTypeStructure, Elements: []mms.TypeSpecElement{
		{Name: "mag", Type: mms.TypeSpec{Type: mms.ValueTypeStructure, Elements: []mms.TypeSpecElement{{Name: "f", Type: mms.TypeSpec{Type: mms.ValueTypeFloat, FormatWidth: 32}}}}},
		{Name: "q", Type: mms.TypeSpec{Type: mms.ValueTypeBitString, Size: 13}},
	}}
	v := mms.NewStructure([]*mms.Value{mms.NewStructure([]*mms.Value{mms.NewFloat(50)}), iec61850.EncodeQuality(0)})
	got := convertValue(v, ts, "Hz")
	if got.Name != "Hz" || len(got.Children) != 2 || got.Children[0].Name != "mag" || got.Children[0].Children[0].Name != "f" ||
		got.Children[1].Quality == nil {
		t.Errorf("structure = %+v", got)
	}
	arr := convertValue(mms.NewArray([]*mms.Value{mms.NewInteger(1), mms.NewInteger(2)}), &mms.TypeSpec{Type: mms.ValueTypeArray, Element: &mms.TypeSpec{Type: mms.ValueTypeInteger}}, "")
	if arr.Type != model.TypeArray || len(arr.Children) != 2 || arr.Text != "[2 elements]" {
		t.Errorf("array = %+v", arr)
	}
	// Nesting deeper than maxValueDepth is cut.
	deep := mms.NewInteger(1)
	for range maxValueDepth + 4 {
		deep = mms.NewStructure([]*mms.Value{deep})
	}
	n, cur := 0, convertValue(deep, nil, "")
	for len(cur.Children) > 0 {
		cur, n = cur.Children[0], n+1
	}
	if n != maxValueDepth {
		t.Errorf("depth = %d, want %d", n, maxValueDepth)
	}
}

func TestClassify(t *testing.T) {
	if classify(nil, false) != nil {
		t.Error("nil")
	}
	for name, tc := range map[string]struct {
		err   error
		code  string
		stage string
	}{
		"rejected association": {mms.ErrConnectionRejected, model.ErrCodeAssociation, stageACSE},
		"negotiation":          {mms.ErrNegotiationFailed, model.ErrCodeAssociation, stageInitiate},
		"not allowed":          {mms.ErrServiceNotAllowed, model.ErrCodeNotAllowed, stageRequest},
		"access denied":        {&mms.DataAccessError{Code: mms.DataAccessErrorObjectAccessDenied}, model.ErrCodeAccessDenied, stageResponse},
		"service error":        {&mms.ServiceError{}, model.ErrCodeServiceError, stageResponse},
		"reject":               {&mms.RejectError{}, model.ErrCodeRejected, stageResponse},
		"closed":               {mms.ErrClosed, model.ErrCodeConnectionLost, ""},
		"decode":               {mms.ErrDecodeFailed, model.ErrCodeDecode, stageResponse},
		"timeout":              {mms.ErrInvokeTimeout, model.ErrCodeTimeout, stageResponse},
		"presentation":         {&mms.ProtocolError{Phase: mms.PhasePresentation}, model.ErrCodeAssociation, stagePresentation},
	} {
		connecting := name == "presentation"
		got := classify(tc.err, connecting)
		if got.Code != tc.code || got.Stage != tc.stage {
			t.Errorf("%s: %+v, want %s/%s", name, got, tc.code, tc.stage)
		}
	}
}
