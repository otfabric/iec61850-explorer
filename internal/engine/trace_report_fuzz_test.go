// SPDX-License-Identifier: MIT

package engine

import (
	"testing"

	"github.com/otfabric/go-mms"
)

// valuesFrom builds report values from fuzz bytes: each value is a kind
// byte and, for strings and bit strings, a length and its bytes. The walker
// sees what a captured InformationReport could carry in any order.
func valuesFrom(b []byte) []*mms.Value {
	var out []*mms.Value
	take := func(n int) []byte {
		n = min(n, len(b))
		p := b[:n]
		b = b[n:]
		return p
	}
	for len(b) > 0 && len(out) < 64 {
		kind := take(1)[0]
		var n int
		if len(b) > 0 {
			n = int(take(1)[0])
		}
		switch kind % 6 {
		case 0:
			out = append(out, mms.NewVisibleString(string(take(n%32))))
		case 1:
			out = append(out, mms.NewInteger(int64(n)-128))
		case 2:
			out = append(out, mms.NewUnsigned(uint64(n)))
		case 3:
			out = append(out, mms.NewBoolean(n%2 == 1))
		case 4:
			bits := n % 80
			out = append(out, mms.NewBitStringWithLength(append([]byte(nil), take((bits+7)/8)...), bits))
		default:
			out = append(out, nil)
		}
	}
	return out
}

// FuzzReportLayout: the walker of a captured report's fields never panics
// and, when it succeeds, keeps its lists consistent with the inclusion bit
// string.
func FuzzReportLayout(f *testing.F) {
	f.Add([]byte{0, 1, 'R', 4, 16, 0x7f, 0xc0, 2, 5})
	f.Add([]byte{0, 1, 'R', 4, 16, 0x08, 0x00, 4, 3, 0xa0, 0, 4, 'L', 'D', '/', 'A', 1, 1, 1, 2})
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, b []byte) {
		r, err := reportLayout(valuesFrom(b))
		if err != nil {
			return
		}
		included := 0
		for _, in := range r.inclusion {
			if in {
				included++
			}
		}
		if len(r.values) != included {
			t.Fatalf("%d values for %d included members", len(r.values), included)
		}
		if len(r.dataRefs) != 0 && len(r.dataRefs) != included {
			t.Fatalf("%d data references for %d included members", len(r.dataRefs), included)
		}
		if len(r.reasons) != 0 && len(r.reasons) != included {
			t.Fatalf("%d reasons for %d included members", len(r.reasons), included)
		}
	})
}
