// SPDX-License-Identifier: MIT

package commission

import (
	"slices"
	"testing"

	"github.com/otfabric/go-mms"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/readonly"
)

// TestServicesFollowCapabilities pins the allow-lists: each capability adds
// only the services it needs, and none adds file changes or ObtainFile.
func TestServicesFollowCapabilities(t *testing.T) {
	never := []mms.Service{mms.ServiceFileRename, mms.ServiceFileDelete, mms.ServiceObtainFile, mms.ServiceUnknown}
	cases := map[string][]mms.Service{
		model.CapWrite:         {mms.ServiceWrite},
		model.CapReports:       {mms.ServiceWrite},
		model.CapSettingGroups: {mms.ServiceWrite},
		model.CapDataSets:      {mms.ServiceDefineNamedVariableList, mms.ServiceDeleteNamedVariableList},
	}
	for c, want := range cases {
		got := Services([]string{c})
		if len(got) != len(readonly.Services)+len(want) {
			t.Errorf("%s: %d services, want %d", c, len(got), len(readonly.Services)+len(want))
		}
		for _, s := range append(slices.Clone(readonly.Services), want...) {
			if !slices.Contains(got, s) {
				t.Errorf("%s: %s missing", c, s)
			}
		}
		for _, s := range never {
			if slices.Contains(got, s) {
				t.Errorf("%s: %s allowed", c, s)
			}
		}
	}
	if got := Services(nil); len(got) != len(readonly.Services) || slices.Contains(got, mms.ServiceWrite) {
		t.Errorf("no capability: %v", got)
	}
	if got := Services([]string{model.CapWrite, model.CapReports, model.CapControls, "laboratory"}); len(got) != len(readonly.Services)+1 {
		t.Errorf("duplicates or unknown capability: %v", got)
	}
	got := Services([]string{model.CapWrite})
	got[0] = mms.ServiceFileDelete
	if readonly.Allowed(mms.ServiceFileDelete) {
		t.Error("Services shares the read-only list")
	}
}

func TestGrantable(t *testing.T) {
	for _, c := range Capabilities() {
		if !Grantable(c) {
			t.Errorf("%s is listed but not grantable", c)
		}
	}
	for _, c := range []string{"laboratory", "", "files"} {
		if Grantable(c) {
			t.Errorf("%q is grantable", c)
		}
	}
	if want := []string{model.CapControls, model.CapDataSets, model.CapReports, model.CapSettingGroups, model.CapWrite}; !slices.Equal(Capabilities(), want) {
		t.Errorf("Capabilities() = %v, want %v", Capabilities(), want)
	}
}
