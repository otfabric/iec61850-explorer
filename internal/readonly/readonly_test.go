// SPDX-License-Identifier: MIT

package readonly

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/otfabric/go-mms"
)

// TestNoStateChangingService pins the list: a service that writes, defines,
// deletes, renames or transfers must never be added to Observe mode. Adding a
// read service is a deliberate change of this test.
func TestNoStateChangingService(t *testing.T) {
	forbidden := []mms.Service{
		mms.ServiceWrite, mms.ServiceDefineNamedVariableList, mms.ServiceDeleteNamedVariableList,
		mms.ServiceFileRename, mms.ServiceFileDelete, mms.ServiceObtainFile, mms.ServiceUnknown,
	}
	for _, s := range forbidden {
		if Allowed(s) {
			t.Errorf("%s is allowed in Observe mode", s)
		}
	}
	for _, s := range []mms.Service{mms.ServiceRead, mms.ServiceGetNameList, mms.ServiceIdentify, mms.ServiceFileRead, mms.ServiceReadJournal} {
		if !Allowed(s) {
			t.Errorf("%s is not allowed", s)
		}
	}
	got := AllowedServices()
	got[0] = mms.ServiceWrite
	if Allowed(mms.ServiceWrite) {
		t.Error("AllowedServices returned the package's own slice")
	}
}

// TestSimIEDOnlyInTests: the programs link no MMS server. The simulated IED
// (internal/simied) is for tests; neither program may depend on it.
func TestSimIEDOnlyInTests(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go list")
	}
	for _, pkg := range []string{"../../cmd/iec61850-explorerd", "../../cmd/iec61850-explorer"} {
		cmd := exec.Command("go", "list", "-deps", pkg)
		cmd.Env = append(os.Environ(), "GOWORK=off")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("go list %s: %v\n%s", pkg, err, out)
		}
		for _, testOnly := range []string{"internal/simied", "internal/proxy/proxytest"} {
			if strings.Contains(string(out), testOnly) {
				t.Errorf("%s depends on %s", pkg, testOnly)
			}
		}
	}
}
