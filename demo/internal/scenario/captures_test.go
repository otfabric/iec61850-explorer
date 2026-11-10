// SPDX-License-Identifier: MIT

package scenario

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/otfabric/go-mms-trace/trace"
)

// want is what go-mms-trace must find in a capture.
type want struct {
	packets    uint64
	assocs     []string       // per association: server open/close
	statuses   map[string]int // service/status -> count
	reports    int
	goose      map[string]string // gocbRef without "AA1" -> "messages/changes/issues"
	sv         string            // "messages/lost/repeated"
	tsduErrors uint64
	unanswered int
	gaps       int
}

var expectations = map[string]want{
	"01-hmi-browse-and-poll.pcapng": {
		packets: 209,
		assocs:  []string{"10.20.1.31:102 associate/conclude", "10.20.1.21:102 associate/conclude", "10.20.1.41:102 associate/conclude"},
		statuses: map[string]int{"GetNameList/ok": 22, "GetNamedVariableListAttributes/ok": 10, "GetVariableAccessAttributes/ok": 15,
			"Identify/ok": 3, "Read/ok": 12},
	},
	"02-reports.pcapng": {
		packets: 67, assocs: []string{"10.20.1.31:102 associate/conclude"},
		statuses: map[string]int{"GetNamedVariableListAttributes/ok": 2, "Identify/ok": 1, "Read/ok": 2, "Write/ok": 10},
		reports:  10,
	},
	"03-control-sbo.pcapng": {
		packets: 76, assocs: []string{"10.20.1.31:102 associate/conclude"},
		statuses: map[string]int{"Read/ok": 4, "Write/ok": 10},
		reports:  11, // 1 GI, LastApplError, 6 for QA1 and its termination, 3 for QB1 and its termination
	},
	"04-settings-files-logs.pcapng": {
		packets: 97, assocs: []string{"10.20.1.31:102 associate/conclude"},
		statuses: map[string]int{"FileClose/ok": 2, "FileDirectory/ok": 1, "FileOpen/ok": 2, "FileRead/ok": 2, "GetNameList/ok": 1,
			"Identify/ok": 1, "Read/ok": 4, "ReadJournal/ok": 1, "Write/ok": 1},
	},
	"05-goose.pcapng": {
		packets: 76,
		goose: map[string]string{"D1Q02MU1MU01/LLN0$GO$gcbStatus": "8/0/", "D1Q02A1PROT/LLN0$GO$gcbTrip": "22/2/", "D1W1A1PROT/LLN0$GO$gcbBF": "22/2/",
			"D1Q02A1CTRL/LLN0$GO$gcbPos": "8/0/", "D1Q03A1PROT/LLN0$GO$gcbTrip": "8/0/", "D1Q03A1CTRL/LLN0$GO$gcbPos": "8/0/"},
	},
	"06-sampled-values.pcapng": {
		packets: 2001, goose: map[string]string{"D1Q02MU1MU01/LLN0$GO$gcbStatus": "1/0/"}, sv: "2000/0/0",
	},
	"07-station-fault.pcapng": {
		packets:  6190,
		assocs:   []string{"10.20.1.31:102 associate/conclude", "10.20.1.41:102 associate/conclude", "10.20.1.21:102 associate/conclude"},
		statuses: map[string]int{"Read/ok": 3, "Write/ok": 9},
		reports:  19,
		goose: map[string]string{"D1Q02MU1MU01/LLN0$GO$gcbStatus": "9/0/", "D1Q02A1PROT/LLN0$GO$gcbTrip": "23/2/", "D1W1A1PROT/LLN0$GO$gcbBF": "23/2/",
			"D1Q02A1CTRL/LLN0$GO$gcbPos": "9/0/", "D1Q03A1PROT/LLN0$GO$gcbTrip": "9/0/", "D1Q03A1CTRL/LLN0$GO$gcbPos": "9/0/"},
		sv: "6000/0/0",
	},
	"08-problems.pcapng": {
		packets: 1504,
		assocs: []string{"10.20.1.41:102 midstream/conclude", "10.20.1.32:102 associate/conclude", "10.20.1.21:102 associate/abort",
			"10.20.1.51:102 associate/conclude"},
		statuses: map[string]int{"/orphan": 1, "GetNamedVariableListAttributes/error": 1, "Identify/ok": 1, "Read/ok": 11, "Read/unanswered": 2},
		goose: map[string]string{"D1Q02MU1MU01/LLN0$GO$gcbStatus": "28/0/stNum decreased", "D1Q02A1PROT/LLN0$GO$gcbTrip": "41/0/repeated (stNum, sqNum)",
			"D1W1A1PROT/LLN0$GO$gcbBF": "40/0/", "D1Q02A1CTRL/LLN0$GO$gcbPos": "40/0/", "D1Q03A1PROT/LLN0$GO$gcbTrip": "40/0/",
			"D1Q03A1CTRL/LLN0$GO$gcbPos": "38/0/sqNum gap"},
		sv:         "1189/12/1",
		tsduErrors: 1, unanswered: 2, gaps: 1,
	},
}

// TestCaptures analyses every capture with go-mms-trace and checks what
// it finds: scenarios 01 to 07 without any error, scenario 08 with each
// of its problems.
func TestCaptures(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "captures", "*.pcapng"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range files {
		names = append(names, filepath.Base(f))
	}
	var all []string
	for _, s := range All() {
		all = append(all, s.File)
	}
	if !slices.Equal(names, all) {
		t.Fatalf("captures %v, want %v", names, all)
	}
	var total int64
	for _, f := range files {
		name := filepath.Base(f)
		t.Run(name, func(t *testing.T) {
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			total += int64(len(b))
			r, err := trace.Analyze(t.Context(), bytes.NewReader(b), trace.Options{})
			if err != nil {
				t.Fatal(err)
			}
			w := expectations[name]
			if r.File.Format != "pcapng" || r.File.Packets != w.packets {
				t.Errorf("%s with %d packets, want pcapng with %d", r.File.Format, r.File.Packets, w.packets)
			}
			if r.File.TruncatedTail || r.File.SkippedBlocks != 0 {
				t.Errorf("file damaged: %+v", r.File)
			}
			c := r.Counters
			if len(c.Unsupported) != 0 || c.TPKTResync != 0 || c.COTPErrors != 0 || c.TSDUErrors != w.tsduErrors ||
				c.BareMMS != 0 || c.GOOSEErrors != 0 || c.SVErrors != 0 {
				t.Errorf("counters %+v", c)
			}
			var assocs []string
			unanswered, gaps := 0, 0
			for _, a := range r.Associations {
				assocs = append(assocs, fmt.Sprintf("%s %s/%s", a.Server, a.Open, a.Close))
				unanswered += a.Unanswered
				gaps += a.Gaps
				if a.Open == "associate" && (a.Accepted == nil || !*a.Accepted || a.Authentication != "none") {
					t.Errorf("association %d: accepted %v, authentication %q", a.ID, a.Accepted, a.Authentication)
				}
			}
			if !slices.Equal(assocs, w.assocs) {
				t.Errorf("associations %v, want %v", assocs, w.assocs)
			}
			if unanswered != w.unanswered || gaps != w.gaps {
				t.Errorf("%d unanswered, %d gaps; want %d, %d", unanswered, gaps, w.unanswered, w.gaps)
			}
			statuses := map[string]int{}
			for _, x := range r.Transactions {
				statuses[x.Service+"/"+x.Status]++
				if x.Request != nil && x.Request.DecodeError != "" || x.Response != nil && x.Response.DecodeError != "" {
					t.Errorf("transaction %d %s: decode error", x.InvokeID, x.Service)
				}
			}
			if len(statuses) == 0 {
				statuses = nil
			}
			if fmt.Sprint(statuses) != fmt.Sprint(w.statuses) {
				t.Errorf("transactions %v, want %v", statuses, w.statuses)
			}
			if len(r.Reports) != w.reports {
				t.Errorf("%d reports, want %d", len(r.Reports), w.reports)
			}
			for _, rep := range r.Reports {
				if rep.PDU == nil || rep.PDU.DecodeError != "" {
					t.Errorf("report in frame %d not decoded", rep.Frame)
				}
			}
			goose := map[string]string{}
			for _, g := range r.GOOSE {
				var issues []string
				for k := range g.Issues {
					issues = append(issues, k)
				}
				slices.Sort(issues)
				goose[strings.TrimPrefix(g.GocbRef, "AA1")] = fmt.Sprintf("%d/%d/%s", g.Messages, g.Changes, strings.Join(issues, ","))
				if g.VLAN != 10 {
					t.Errorf("%s on VLAN %d, want 10", g.GocbRef, g.VLAN)
				}
			}
			if len(goose) == 0 {
				goose = nil
			}
			if fmt.Sprint(goose) != fmt.Sprint(w.goose) {
				t.Errorf("GOOSE %v, want %v", goose, w.goose)
			}
			sv := ""
			for _, s := range r.SV {
				sv = fmt.Sprintf("%d/%d/%d", s.Messages, s.Lost, s.Repeated)
				if s.VLAN != 20 || s.AppID != 0x4001 || s.Wrap > 4000 {
					t.Errorf("SV stream %+v", s)
				}
			}
			if sv != w.sv {
				t.Errorf("SV %s, want %s", sv, w.sv)
			}
		})
	}
	if total > 5<<20 {
		t.Errorf("captures take %d bytes, more than 5 MiB", total)
	}
}
