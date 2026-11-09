// SPDX-License-Identifier: MIT

package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/otfabric/iec61850-explorer/internal/model"
)

// corpusBudget bounds the offline workflow on one IED of a corpus file:
// open, browse every node, search, list data sets and report control
// blocks, export a snapshot, open the IED a second time and compare the
// two. The largest corpus files (2-3 MB vendor CIDs) take well under a
// second.
const corpusBudget = 5 * time.Second

// TestSCLCorpusOffline opens every IED of every file of the OT Fabric SCL
// corpus as an offline session and runs the offline workflow on it. The
// corpus is private: the test runs when IEC61850_SCL_CORPUS names a
// checkout of otfabric/iec61850-scl-corpus and skips otherwise;
// IEC61850_SCL_CORPUS_REQUIRE=1 turns the skip into a failure.
//
// Files the corpus manifest records as unparsable must be refused with an
// error, not a panic; every other file must open for each IED that has an
// MMS server, within corpusBudget.
func TestSCLCorpusOffline(t *testing.T) {
	root := os.Getenv("IEC61850_SCL_CORPUS")
	if root == "" {
		if os.Getenv("IEC61850_SCL_CORPUS_REQUIRE") == "1" {
			t.Fatal("IEC61850_SCL_CORPUS_REQUIRE=1 but IEC61850_SCL_CORPUS is not set")
		}
		t.Skip("IEC61850_SCL_CORPUS is not set (a checkout of otfabric/iec61850-scl-corpus)")
	}
	raw, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Files []struct {
			Path     string `json:"path"`
			Expected struct {
				Parsed bool `json:"parsed"`
			} `json:"expected"`
		} `json:"files"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}

	type timing struct {
		name  string
		nodes int
		took  time.Duration
	}
	var timings []timing
	e := New(Options{MaxSessions: 4})
	t.Cleanup(e.Close)
	ctx := t.Context()

	for _, f := range m.Files {
		t.Run(f.Path, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(root, f.Path))
			if err != nil {
				t.Fatal(err)
			}
			file := model.File{Name: filepath.Base(f.Path), Data: data}
			info, err := e.InspectSCL(ctx, file)
			if !f.Expected.Parsed {
				if err == nil {
					t.Errorf("unparsable file inspected: %+v", info)
				}
				return
			}
			if err != nil {
				t.Fatalf("InspectSCL: %v", err)
			}
			for _, ied := range info.IEDs {
				for _, ap := range ied.AccessPoints {
					start := time.Now()
					s, err := e.OpenSCL(ctx, model.OpenSCLRequest{File: file, IED: ied.Name, AccessPoint: ap.Name})
					if err != nil {
						t.Errorf("OpenSCL %s/%s: %v", ied.Name, ap.Name, err)
						continue
					}
					nodes := browseAll(t, e, s.ID, "")
					if _, err := e.Search(ctx, s.ID, model.SearchQuery{Text: "LLN0"}); err != nil {
						t.Errorf("Search: %v", err)
					}
					lds, _ := e.Browse(ctx, s.ID, "")
					for _, ld := range lds {
						if _, err := e.DataSets(ctx, s.ID, ld.Ref); err != nil {
							t.Errorf("DataSets %s: %v", ld.Ref, err)
						}
						if _, err := e.Reports(ctx, s.ID, ld.Ref); err != nil {
							t.Errorf("Reports %s: %v", ld.Ref, err)
						}
					}
					if _, err := e.Export(ctx, s.ID, model.ExportRequest{Format: model.ExportSnapshot}); err != nil {
						t.Errorf("Export: %v", err)
					}
					// The same IED opened twice compares equal.
					if twin, err := e.OpenSCL(ctx, model.OpenSCLRequest{File: file, IED: ied.Name, AccessPoint: ap.Name}); err != nil {
						t.Errorf("OpenSCL twin: %v", err)
					} else {
						if c, err := e.Compare(ctx, model.CompareRequest{A: s.ID, B: twin.ID, Values: true}); err != nil || len(c.Differences) != 0 {
							t.Errorf("Compare with itself: %v, %d differences", err, len(c.Differences))
						}
						_ = e.CloseSession(ctx, twin.ID)
					}
					took := time.Since(start)
					if err := e.CloseSession(ctx, s.ID); err != nil {
						t.Errorf("CloseSession: %v", err)
					}
					if took > corpusBudget {
						t.Errorf("%s/%s took %s (%d nodes), budget %s", ied.Name, ap.Name, took, nodes, corpusBudget)
					}
					timings = append(timings, timing{f.Path + " " + ied.Name + "/" + ap.Name, nodes, took})
				}
			}
		})
	}

	sort.Slice(timings, func(i, j int) bool { return timings[i].took > timings[j].took })
	total := 0
	for _, tm := range timings {
		total += tm.nodes
	}
	t.Logf("%d IED sessions, %d nodes browsed", len(timings), total)
	for _, tm := range timings[:min(5, len(timings))] {
		t.Logf("slowest: %-80s %6d nodes %v", tm.name, tm.nodes, tm.took.Round(time.Millisecond))
	}
}

// browseAll walks the whole tree of a session and returns the node count.
func browseAll(t *testing.T, e *Engine, id, ref string) int {
	t.Helper()
	nodes, err := e.Browse(t.Context(), id, ref)
	if err != nil {
		t.Errorf("Browse %q: %v", ref, err)
		return 0
	}
	n := len(nodes)
	for _, c := range nodes {
		if c.Children > 0 {
			n += browseAll(t, e, id, c.Ref)
		}
	}
	return n
}
