// SPDX-License-Identifier: MIT

package engine

import (
	"errors"
	"fmt"

	iec61850 "github.com/otfabric/go-iec61850"
	"github.com/otfabric/go-mms"
)

// capturedReport is the layout of a captured IEC 61850 report: the values
// of an InformationReport on the variable list RPT, in the order IEC
// 61850-8-1 (clause 17.2) gives them. Each value was decoded by go-mms;
// this only says which value is which field, the way go-iec61850's client
// does for the reports it subscribes to (its decoder is not exported).
type capturedReport struct {
	rptID     string
	optFlds   iec61850.OptFlds
	seqNum    uint32
	datSet    string
	confRev   uint32
	inclusion []bool
	dataRefs  []string
	values    []*mms.Value
	reasons   []iec61850.ReasonCode
}

var errReportShort = errors.New("fewer values than the optional fields and inclusion bit string announce")

// reportLayout walks the values of a report.
func reportLayout(values []*mms.Value) (*capturedReport, error) {
	r := &capturedReport{}
	i := 0
	next := func() (*mms.Value, error) {
		if i >= len(values) {
			return nil, errReportShort
		}
		i++
		if values[i-1] == nil {
			// go-mms gives a failed result as a DataAccessError value,
			// never nil; a nil value is refused all the same.
			return nil, fmt.Errorf("value %d is missing", i)
		}
		return values[i-1], nil
	}
	v, err := next()
	if err != nil {
		return nil, err
	}
	var ok bool
	if r.rptID, ok = v.VisibleString(); !ok {
		return nil, fmt.Errorf("the first value (RptID) is a %v, not a visible string", v.Type())
	}
	if v, err = next(); err != nil {
		return nil, err
	}
	bits, err := bitsOf(v, 10)
	if err != nil {
		return nil, fmt.Errorf("OptFlds: %w", err)
	}
	// Bit 0 is reserved; bits 1 to 9 are go-iec61850's OptFlds 0 to 8.
	for b := 1; b <= 9; b++ {
		if bits[b] {
			r.optFlds |= 1 << (b - 1)
		}
	}
	type field struct {
		flag iec61850.OptFlds
		take func(*mms.Value)
	}
	fields := []field{
		{iec61850.OptFldSeqNum, func(v *mms.Value) { r.seqNum, _ = v.Uint32() }},
		{iec61850.OptFldTimeStamp, func(*mms.Value) {}},
		{iec61850.OptFldDataSet, func(v *mms.Value) { r.datSet, _ = v.VisibleString() }},
		{iec61850.OptFldBufOvfl, func(*mms.Value) {}},
		{iec61850.OptFldEntryID, func(*mms.Value) {}},
		{iec61850.OptFldConfRev, func(v *mms.Value) { r.confRev, _ = v.Uint32() }},
		{iec61850.OptFldSegmentation, func(*mms.Value) {}},
	}
	for _, f := range fields {
		if !r.optFlds.Has(f.flag) {
			continue
		}
		n := 1
		if f.flag == iec61850.OptFldSegmentation {
			n = 2 // SubSeqNum, MoreSegmentsFollow
		}
		for range n {
			if v, err = next(); err != nil {
				return nil, err
			}
			f.take(v)
		}
	}
	if v, err = next(); err != nil {
		return nil, err
	}
	if r.inclusion, err = bitsOf(v, 0); err != nil {
		return nil, fmt.Errorf("inclusion: %w", err)
	}
	included := 0
	for _, b := range r.inclusion {
		if b {
			included++
		}
	}
	if r.optFlds.Has(iec61850.OptFldDataRef) {
		for range included {
			if v, err = next(); err != nil {
				return nil, err
			}
			s, _ := v.VisibleString()
			r.dataRefs = append(r.dataRefs, s)
		}
	}
	for range included {
		if v, err = next(); err != nil {
			return nil, err
		}
		r.values = append(r.values, v)
	}
	if r.optFlds.Has(iec61850.OptFldReasonCode) {
		for range included {
			if v, err = next(); err != nil {
				return nil, err
			}
			rb, err := bitsOf(v, 0)
			if err != nil {
				return nil, fmt.Errorf("reason code: %w", err)
			}
			// Bit 0 is reserved; the reasons start at bit 1.
			var rc iec61850.ReasonCode
			for b := 1; b < len(rb) && b <= 7; b++ {
				if rb[b] {
					rc |= 1 << (b - 1)
				}
			}
			r.reasons = append(r.reasons, rc)
		}
	}
	if i != len(values) {
		return nil, fmt.Errorf("%d values left over after the fields the report announces", len(values)-i)
	}
	return r, nil
}

// bitsOf returns the bits of a bit string value, which must have at least
// minBits.
func bitsOf(v *mms.Value, minBits int) ([]bool, error) {
	data, ok := v.BitString()
	if !ok {
		return nil, fmt.Errorf("a %v, not a bit string", v.Type())
	}
	n, ok := v.BitStringLength()
	if !ok {
		n = 8 * len(data)
	}
	if n < minBits || n > 8*len(data) {
		return nil, fmt.Errorf("%d bits", n)
	}
	out := make([]bool, n)
	for b := range n {
		out[b] = data[b/8]&(0x80>>(b%8)) != 0
	}
	return out, nil
}
