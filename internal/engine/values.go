// SPDX-License-Identifier: MIT

package engine

import (
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	iec61850 "github.com/otfabric/go-iec61850"
	"github.com/otfabric/go-mms"

	"github.com/otfabric/iec61850-explorer/internal/model"
)

// maxValueDepth bounds the nesting of a converted value. IEC 61850 values
// are a few levels deep; a deeper one is cut, not followed.
const maxValueDepth = 16

// convertValue turns an MMS value into its DTO. ts, when not nil, is the type
// of the variable from GetVariableAccessAttributes and supplies the component
// names of structures. name is the component name of v.
func convertValue(v *mms.Value, ts *mms.TypeSpec, name string) model.Value {
	return convert(v, ts, name, 0)
}

func convert(v *mms.Value, ts *mms.TypeSpec, name string, depth int) model.Value {
	out := model.Value{Name: name}
	if v == nil {
		out.Type, out.Text = model.TypeOther, "no value"
		return out
	}
	switch v.Type() {
	case mms.ValueTypeBoolean:
		b, _ := v.Bool()
		out.Type, out.Text, out.JSON = model.TypeBoolean, strconv.FormatBool(b), b
	case mms.ValueTypeInteger:
		i, _ := v.Int64()
		out.Type, out.Text, out.JSON = model.TypeInteger, strconv.FormatInt(i, 10), jsonInt(i)
	case mms.ValueTypeUnsigned:
		u, _ := v.Uint64()
		out.Type, out.Text = model.TypeUnsigned, strconv.FormatUint(u, 10)
		if u <= 1<<53 {
			out.JSON = u
		} else {
			out.JSON = out.Text
		}
	case mms.ValueTypeFloat:
		f, _ := v.Float64()
		// A FLOAT32 is shown with the digits a float32 has: 230.67, not
		// 230.66998. Servers describe a single float as format width 32 or
		// as 24 (the mantissa, ISO 9506-2 FloatingPoint); without a type, a
		// value that a float32 holds exactly is taken as one.
		bits := 64
		switch {
		case ts != nil && ts.Type == mms.ValueTypeFloat && ts.FormatWidth > 0:
			if ts.FormatWidth <= 32 {
				bits = 32
			}
		case float64(float32(f)) == f:
			bits = 32
		}
		out.Type, out.Text = model.TypeFloat, strconv.FormatFloat(f, 'g', -1, bits)
		if !math.IsNaN(f) && !math.IsInf(f, 0) {
			out.JSON = f
		}
	case mms.ValueTypeBitString:
		b, _ := v.BitString()
		n, _ := v.BitStringLength()
		out.Type, out.Bits, out.Text = model.TypeBitString, n, bitText(b, n)
		out.JSON = out.Text
		if name == "q" && n == 13 {
			if q, err := iec61850.DecodeQuality(v); err == nil {
				out.Quality = qualityOf(q)
				out.Text = qualityText(out.Quality)
			}
		}
	case mms.ValueTypeOctetString:
		b, _ := v.OctetString()
		out.Type, out.Text = model.TypeOctetString, hex.EncodeToString(b)
		out.JSON = out.Text
	case mms.ValueTypeVisibleString:
		s, _ := v.VisibleString()
		out.Type, out.Text, out.JSON = model.TypeVisible, s, s
	case mms.ValueTypeMmsString:
		s, _ := v.MmsString()
		out.Type, out.Text, out.JSON = model.TypeMMSString, s, s
	case mms.ValueTypeUTCTime:
		out.Type = model.TypeUTCTime
		if ts, err := iec61850.DecodeTimestamp(v); err == nil {
			out.Time = &model.Timestamp{
				Time:                 ts.Time.UTC(),
				LeapSecondKnown:      ts.Quality.LeapSecondKnown,
				ClockFailure:         ts.Quality.ClockFailure,
				ClockNotSynchronized: ts.Quality.ClockNotSynchronized,
				Accuracy:             ts.Quality.TimeAccuracy,
			}
			out.Text = ts.Time.UTC().Format("2006-01-02T15:04:05.000000Z")
			out.JSON = out.Text
		} else {
			out.Text = v.String()
		}
	case mms.ValueTypeBinaryTime:
		ms, _ := v.BinaryTime()
		out.Type, out.Text, out.JSON = model.TypeBinaryTime, strconv.FormatInt(ms, 10)+" ms", ms
	case mms.ValueTypeStructure:
		elems, _ := v.Structure()
		out.Type = model.TypeStructure
		out.Children = convertChildren(elems, ts, depth, true)
		out.Text = fmt.Sprintf("{%d components}", len(elems))
	case mms.ValueTypeArray:
		elems, _ := v.ArrayElements()
		out.Type = model.TypeArray
		out.Children = convertChildren(elems, ts, depth, false)
		out.Text = fmt.Sprintf("[%d elements]", len(elems))
	case mms.ValueTypeDataAccessError:
		code, _ := v.DataAccessErr()
		out.Type, out.Text = model.TypeAccessError, code.String()
	default:
		out.Type, out.Text = model.TypeOther, v.String()
	}
	return out
}

func convertChildren(elems []*mms.Value, ts *mms.TypeSpec, depth int, structure bool) []model.Value {
	if depth >= maxValueDepth {
		return nil
	}
	out := make([]model.Value, len(elems))
	for i, el := range elems {
		var (
			childType *mms.TypeSpec
			childName string
		)
		switch {
		case structure && ts != nil && ts.Type == mms.ValueTypeStructure && i < len(ts.Elements):
			childType, childName = &ts.Elements[i].Type, ts.Elements[i].Name
		case !structure && ts != nil && ts.Type == mms.ValueTypeArray:
			childType = ts.Element
		}
		out[i] = convert(el, childType, childName, depth+1)
	}
	return out
}

// jsonInt keeps an integer exact in JSON: beyond 2^53 a float64 (which is
// what JavaScript reads) loses digits, so the number goes as a string.
func jsonInt(i int64) any {
	if i > 1<<53 || i < -(1<<53) {
		return strconv.FormatInt(i, 10)
	}
	return i
}

// bitText shows a bit string as its bits, first bit first.
func bitText(b []byte, n int) string {
	var sb strings.Builder
	for i := 0; i < n && i/8 < len(b); i++ {
		if b[i/8]&(0x80>>(i%8)) != 0 {
			sb.WriteByte('1')
		} else {
			sb.WriteByte('0')
		}
	}
	return sb.String()
}

var qualityFlags = []struct {
	bit  iec61850.Quality
	name string
}{
	{iec61850.QualityOverflow, "overflow"},
	{iec61850.QualityOutOfRange, "out-of-range"},
	{iec61850.QualityBadReference, "bad-reference"},
	{iec61850.QualityOscillatory, "oscillatory"},
	{iec61850.QualityFailure, "failure"},
	{iec61850.QualityOldData, "old-data"},
	{iec61850.QualityInconsistent, "inconsistent"},
	{iec61850.QualityInaccurate, "inaccurate"},
	{iec61850.QualitySourceSubstituted, "substituted"},
	{iec61850.QualityTest, "test"},
	{iec61850.QualityOperatorBlocked, "operator-blocked"},
}

func qualityOf(q iec61850.Quality) *model.Quality {
	out := &model.Quality{Raw: uint16(q), Flags: []string{}}
	switch q.Validity() {
	case iec61850.ValidityGood:
		out.Validity = "good"
	case iec61850.ValidityInvalid:
		out.Validity = "invalid"
	case iec61850.ValidityQuestionable:
		out.Validity = "questionable"
	default:
		out.Validity = "reserved"
	}
	for _, f := range qualityFlags {
		if q&f.bit != 0 {
			out.Flags = append(out.Flags, f.name)
		}
	}
	return out
}

func qualityText(q *model.Quality) string {
	if len(q.Flags) == 0 {
		return q.Validity
	}
	return q.Validity + " (" + strings.Join(q.Flags, ", ") + ")"
}

// since returns the milliseconds from start to now.
func since(start time.Time) int64 {
	return time.Since(start).Milliseconds()
}
