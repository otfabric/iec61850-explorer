// SPDX-License-Identifier: MIT

package engine

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/otfabric/go-mms"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
)

// Selector length limits: a T-selector is at most 4 octets (IEC 61850-8-1
// §6.2.3.2.2, enforced by go-mms); session and presentation selectors at
// most 16 (ISO 8327-1, ISO 8823-1).
const (
	maxTSelector = 4
	maxSPSelector = 16
	maxAPTitleArcs = 16
)

// applyAddressing sets the ISO addressing of a target on dial options. A
// nil field keeps go-mms's default; "" omits the parameter.
func applyAddressing(a *model.Addressing, o *mms.DialOptions) error {
	if a == nil {
		return nil
	}
	sel := func(name string, v *string, limit int, dst *[]byte) error {
		if v == nil {
			return nil
		}
		b, err := parseSelector(*v, limit)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		*dst = b
		return nil
	}
	title := func(name string, v *string, dst *mms.APTitle) error {
		if v == nil {
			return nil
		}
		t, err := parseAPTitle(*v)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		*dst = t
		return nil
	}
	qual := func(name string, v *int, dst *int) error {
		if v == nil {
			return nil
		}
		if *v < 0 || *v > 1<<31-1 {
			return fmt.Errorf("%s: %d is out of range", name, *v)
		}
		*dst = *v
		return nil
	}
	for _, err := range []error{
		sel("local T-selector", a.LocalTSelector, maxTSelector, &o.Transport.LocalTSelector),
		sel("remote T-selector", a.RemoteTSelector, maxTSelector, &o.Transport.RemoteTSelector),
		sel("local S-selector", a.LocalSSelector, maxSPSelector, &o.ISO.LocalSSelector),
		sel("remote S-selector", a.RemoteSSelector, maxSPSelector, &o.ISO.RemoteSSelector),
		sel("local P-selector", a.LocalPSelector, maxSPSelector, &o.ISO.LocalPSelector),
		sel("remote P-selector", a.RemotePSelector, maxSPSelector, &o.ISO.RemotePSelector),
		title("local AP-title", a.LocalAPTitle, &o.ISO.LocalAPTitle),
		title("remote AP-title", a.RemoteAPTitle, &o.ISO.RemoteAPTitle),
		qual("local AE-qualifier", a.LocalAEQualifier, &o.ISO.LocalAEQualifier),
		qual("remote AE-qualifier", a.RemoteAEQualifier, &o.ISO.RemoteAEQualifier),
	} {
		if err != nil {
			return service.Errorf(service.CodeInvalidArgument, "%v", err)
		}
	}
	return nil
}

// parseSelector reads a hexadecimal selector. "" is an empty, non-nil
// selector: the parameter is omitted.
func parseSelector(s string, limit int) ([]byte, error) {
	s = strings.TrimPrefix(strings.ReplaceAll(strings.TrimSpace(s), " ", ""), "0x")
	b, err := hex.DecodeString(s)
	switch {
	case err != nil:
		return nil, fmt.Errorf("%q is not hexadecimal", s)
	case len(b) > limit:
		return nil, fmt.Errorf("%d octets, at most %d", len(b), limit)
	}
	return b, nil
}

// parseAPTitle reads an object identifier, 1.3.9999.23 or 1,3,9999,23. ""
// is an empty, non-nil AP-title: the parameter is omitted.
func parseAPTitle(s string) (mms.APTitle, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return mms.APTitle{}, nil
	}
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == '.' || r == ',' })
	if len(parts) < 2 || len(parts) > maxAPTitleArcs {
		return nil, fmt.Errorf("%q: an object identifier has 2 to %d arcs", s, maxAPTitleArcs)
	}
	out := make(mms.APTitle, len(parts))
	for i, p := range parts {
		n, err := strconv.ParseUint(strings.TrimSpace(p), 10, 31)
		if err != nil {
			return nil, fmt.Errorf("%q: arc %q is not a number", s, p)
		}
		out[i] = int(n)
	}
	// X.660: the first arc is 0, 1 or 2; under 0 and 1 the second is below 40.
	if out[0] > 2 || (out[0] < 2 && out[1] >= 40) {
		return nil, fmt.Errorf("%q is not a valid object identifier", s)
	}
	return out, nil
}
