// SPDX-License-Identifier: MIT

package engine

import (
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/otfabric/go-mms"
)

// parseValue turns the text of a write into a value of the attribute's MMS
// type. Only scalar attributes are written; a structure or array is
// written attribute by attribute.
func parseValue(text string, ts *mms.TypeSpec) (*mms.Value, error) {
	if ts == nil {
		return nil, fmt.Errorf("the type of the attribute is unknown")
	}
	raw := text
	text = strings.TrimSpace(text)
	switch ts.Type {
	case mms.ValueTypeBoolean:
		b, err := strconv.ParseBool(text)
		if err != nil {
			return nil, fmt.Errorf("%q is not true or false", text)
		}
		return mms.NewBoolean(b), nil
	case mms.ValueTypeInteger:
		bits := intBits(ts.Size)
		i, err := strconv.ParseInt(text, 10, bits)
		if err != nil {
			return nil, fmt.Errorf("%q is not an INT%d", text, bits)
		}
		return mms.NewInteger(i), nil
	case mms.ValueTypeUnsigned:
		bits := intBits(ts.Size)
		u, err := strconv.ParseUint(text, 10, bits)
		if err != nil {
			return nil, fmt.Errorf("%q is not an INT%dU", text, bits)
		}
		return mms.NewUnsigned(u), nil
	case mms.ValueTypeFloat:
		single := ts.FormatWidth > 0 && ts.FormatWidth <= 32
		bits := 64
		if single {
			bits = 32
		}
		f, err := strconv.ParseFloat(text, bits)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, fmt.Errorf("%q is not a finite FLOAT%d", text, bits)
		}
		if single {
			f = float64(float32(f)) // sent as a single float
		}
		return mms.NewFloat(f), nil
	case mms.ValueTypeVisibleString, mms.ValueTypeMmsString:
		if err := checkSize(len(raw), ts.Size, "characters"); err != nil {
			return nil, err
		}
		if ts.Type == mms.ValueTypeVisibleString {
			for _, r := range raw {
				if r < 0x20 || r > 0x7e {
					return nil, fmt.Errorf("a VisibleString holds printable ASCII only")
				}
			}
			return mms.NewVisibleString(raw), nil
		}
		return mms.NewMmsString(raw), nil
	case mms.ValueTypeBitString:
		if strings.Trim(text, "01") != "" || text == "" {
			return nil, fmt.Errorf("%q is not a bit string such as 0101", text)
		}
		if ts.Size > 0 && len(text) != ts.Size {
			return nil, fmt.Errorf("the bit string has %d bits, not %d", ts.Size, len(text))
		}
		if err := checkSize(len(text), ts.Size, "bits"); err != nil {
			return nil, err
		}
		b := make([]byte, (len(text)+7)/8)
		for i, c := range text {
			if c == '1' {
				b[i/8] |= 0x80 >> (i % 8)
			}
		}
		return mms.NewBitStringWithLength(b, len(text)), nil
	case mms.ValueTypeOctetString:
		b, err := hex.DecodeString(strings.ReplaceAll(text, " ", ""))
		if err != nil {
			return nil, fmt.Errorf("%q is not hexadecimal", text)
		}
		if ts.Size > 0 && len(b) != ts.Size {
			return nil, fmt.Errorf("the octet string has %d octets, not %d", ts.Size, len(b))
		}
		if err := checkSize(len(b), ts.Size, "octets"); err != nil {
			return nil, err
		}
		return mms.NewOctetString(b), nil
	case mms.ValueTypeUTCTime:
		t, err := time.Parse(time.RFC3339Nano, text)
		if err != nil {
			return nil, fmt.Errorf("%q is not an RFC 3339 time such as 2026-11-09T12:00:00Z", text)
		}
		return mms.NewUTCTime(t.UTC()), nil
	default:
		return nil, fmt.Errorf("a %s is not written as one value; write its attributes", ts.Type)
	}
}

// intBits is the bit width of an INT or INTU type; 0 (unknown) is taken as
// 64 and the IED checks the range.
func intBits(size int) int {
	switch size {
	case 8, 16, 24, 32, 64:
		return size
	}
	return 64
}

// checkSize applies the size of a string type: a negative size is a
// maximum (variable length), a positive one exact or a maximum depending on
// the type and checked by the caller; zero is unbounded.
func checkSize(n, size int, unit string) error {
	if size < 0 && n > -size {
		return fmt.Errorf("at most %d %s", -size, unit)
	}
	if size > 0 && n > size {
		return fmt.Errorf("at most %d %s", size, unit)
	}
	return nil
}
