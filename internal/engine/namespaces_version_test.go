// SPDX-License-Identifier: MIT

package engine

import (
	"strconv"
	"strings"
	"testing"

	"github.com/otfabric/iec61850-explorer/internal/buildinfo"
)

// minNamespaceImporter is the oldest go-iec61850-model release whose NSD
// importer the engine may use: from v0.10.2 on, the time and memory of an
// import grow in proportion to the package (go-iec61850-model RELEASE.md,
// v0.10.2), and v0.10.3 is the release the application requires.
const minNamespaceImporter = "v0.10.3"

// AddNamespaces and the start of the engine import packages the user chose
// with go-iec61850-model; the binary must carry a release of it at least
// minNamespaceImporter.
func TestNamespaceImporterVersion(t *testing.T) {
	v := buildinfo.ModuleVersion("github.com/otfabric/go-iec61850-model")
	got, ok := releaseOf(v)
	if !ok {
		t.Skipf("go-iec61850-model is %q here, not a release (a workspace or a replaced module)", v)
	}
	want, _ := releaseOf(minNamespaceImporter)
	for i := range got {
		if got[i] != want[i] {
			if got[i] < want[i] {
				t.Fatalf("built with go-iec61850-model %s, the namespace import needs %s or later", v, minNamespaceImporter)
			}
			return
		}
	}
}

// releaseOf parses "vX.Y.Z" (a pre-release suffix is ignored).
func releaseOf(v string) ([3]int, bool) {
	var out [3]int
	core, _, _ := strings.Cut(strings.TrimPrefix(v, "v"), "-")
	parts := strings.Split(core, ".")
	if !strings.HasPrefix(v, "v") || len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
