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
// deletes, renames or transfers must never be added to Observe mode. File and
// journal reads do not change state but are not used yet; adding them is a
// deliberate change of this test.
func TestNoStateChangingService(t *testing.T) {
	forbidden := []mms.Service{
		// State-changing.
		mms.ServiceWrite, mms.ServiceDefineNamedVariableList, mms.ServiceDeleteNamedVariableList,
		mms.ServiceFileRename, mms.ServiceFileDelete, mms.ServiceObtainFile, mms.ServiceUnknown,
		// Read-only, not used in M0.
		mms.ServiceFileOpen, mms.ServiceFileRead, mms.ServiceFileClose, mms.ServiceFileDirectory,
		mms.ServiceReadJournal,
	}
	for _, s := range forbidden {
		if Allowed(s) {
			t.Errorf("%s is allowed in Observe mode", s)
		}
	}
	for _, s := range []mms.Service{mms.ServiceRead, mms.ServiceGetNameList, mms.ServiceIdentify} {
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
		if strings.Contains(string(out), "internal/simied") {
			t.Errorf("%s depends on internal/simied", pkg)
		}
	}
}
