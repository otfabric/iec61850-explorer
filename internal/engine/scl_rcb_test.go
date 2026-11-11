// SPDX-License-Identifier: MIT

package engine

import (
	"slices"
	"testing"

	"github.com/otfabric/go-iec61850/scl"

	"github.com/otfabric/iec61850-explorer/internal/model"
)

func TestRCBInstanceNames(t *testing.T) {
	for _, tc := range []struct {
		rc   scl.ReportControl
		want []string
	}{
		{scl.ReportControl{Name: "urcb", Indexed: false, MaxInstances: 3}, []string{"urcb"}},
		{scl.ReportControl{Name: "urcb", Indexed: true}, []string{"urcb01"}},
		{scl.ReportControl{Name: "urcb01", Indexed: true, MaxInstances: 2}, []string{"urcb0101", "urcb0102"}},
	} {
		if got := rcbInstanceNames(tc.rc); !slices.Equal(got, tc.want) {
			t.Errorf("rcbInstanceNames(%+v) = %v, want %v", tc.rc, got, tc.want)
		}
	}
	if got := rcbInstanceNames(scl.ReportControl{Name: "r", Indexed: true, MaxInstances: 500}); len(got) != 99 || got[98] != "r99" {
		t.Errorf("capped instances = %d, last %q", len(got), got[len(got)-1])
	}
}

func TestComparedOptFlds(t *testing.T) {
	all := []string{"seq-num", "buf-ovfl", "entry-id"}
	if got := comparedOptFlds(model.RCB{OptFlds: all}); !slices.Equal(got, []string{"seq-num"}) {
		t.Errorf("URCB = %v", got)
	}
	if got := comparedOptFlds(model.RCB{Buffered: true, OptFlds: all}); !slices.Equal(got, all) {
		t.Errorf("BRCB = %v", got)
	}
}
