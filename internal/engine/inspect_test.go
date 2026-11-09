// SPDX-License-Identifier: MIT

package engine

import (
	"bytes"
	"slices"
	"testing"
	"time"

	"github.com/otfabric/go-mms"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/readonly"
	"github.com/otfabric/iec61850-explorer/internal/service"
	"github.com/otfabric/iec61850-explorer/internal/simied"
)

// The data sets an IED reports are the data sets its SCL file defines.
func TestDataSetsLiveMatchSCL(t *testing.T) {
	ied := startIED(t)
	e := newEngine(t)
	ctx := t.Context()
	live, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	offline, err := e.OpenSCL(ctx, model.OpenSCLRequest{File: model.File{Name: "demo.icd", Data: simied.ICD()}})
	if err != nil {
		t.Fatal(err)
	}
	a, err := e.DataSets(ctx, live.ID, simied.Domain)
	if err != nil {
		t.Fatal(err)
	}
	b, err := e.DataSets(ctx, offline.ID, simied.Domain)
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 1 || len(b) != 1 || a[0].Ref != simied.Domain+"/LLN0.Meas" || a[0].Source != model.SourceIED || b[0].Source != model.SourceSCL {
		t.Fatalf("live %+v\nscl %+v", a, b)
	}
	if !slices.EqualFunc(a[0].Members, b[0].Members, func(x, y model.DataSetMember) bool { return x.Ref == y.Ref && x.MMS == y.MMS }) {
		t.Errorf("members live %+v\nscl %+v", a[0].Members, b[0].Members)
	}
	if a[0].MMS != (model.MMSName{Domain: simied.Domain, ItemID: "LLN0$Meas"}) || b[0].Desc != "Measurements" {
		t.Errorf("names = %+v / %+v", a[0].MMS, b[0])
	}

	before := ied.Services()[mms.ServiceRead]
	res, err := e.ReadDataSet(ctx, live.ID, a[0].Ref)
	if err != nil {
		t.Fatal(err)
	}
	if n := ied.Services()[mms.ServiceRead] - before; n != 1 {
		t.Errorf("%d reads for a data set", n)
	}
	if len(res.Values) != 2 || res.Values[0].Value == nil || res.Values[0].Ref != simied.Domain+"/MMXU1.Hz[MX]" {
		t.Errorf("data set values = %+v", res)
	}
	if _, err := e.ReadDataSet(ctx, live.ID, "nope"); service.CodeOf(err) != service.CodeInvalidArgument {
		t.Errorf("bad ref = %v", err)
	}
	if _, err := e.ReadDataSet(ctx, live.ID, simied.Domain+"/LLN0.Nope"); service.CodeOf(err) != service.CodeIEDError {
		t.Errorf("unknown data set = %v", err)
	}
	if _, err := e.DataSets(ctx, live.ID, "NoSuchLD"); service.CodeOf(err) != service.CodeNotFound {
		t.Errorf("unknown LD = %v", err)
	}
	if _, err := e.ReadDataSet(ctx, offline.ID, a[0].Ref); service.CodeOf(err) != service.CodeNotAllowed {
		t.Errorf("read offline data set = %v", err)
	}
}

// The configuration of a report control block, as the IED reports it and
// as its SCL file defines it.
func TestReportsLiveMatchSCL(t *testing.T) {
	ied := startIED(t)
	e := newEngine(t)
	ctx := t.Context()
	live, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	offline, err := e.OpenSCL(ctx, model.OpenSCLRequest{File: model.File{Name: "demo.icd", Data: simied.ICD()}})
	if err != nil {
		t.Fatal(err)
	}
	a, err := e.Reports(ctx, live.ID, simied.Domain)
	if err != nil {
		t.Fatal(err)
	}
	b, err := e.Reports(ctx, offline.ID, simied.Domain)
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 1 || len(b) != 1 {
		t.Fatalf("live %+v\nscl %+v", a, b)
	}
	l, s := a[0], b[0]
	if l.Error != nil || l.RptEna == nil || *l.RptEna || l.Buffered || l.Resv == nil {
		t.Errorf("live = %+v", l)
	}
	if l.Ref != s.Ref || l.MMS != s.MMS || l.RptID != s.RptID || l.DatSet != s.DatSet || l.ConfRev != s.ConfRev ||
		!slices.Equal(l.TrgOps, s.TrgOps) || !slices.Equal(l.OptFlds, s.OptFlds) {
		t.Errorf("live %+v\nscl  %+v", l, s)
	}
	if s.RptEna != nil || s.Source != model.SourceSCL {
		t.Errorf("SCL RCB claims IED state: %+v", s)
	}
}

func TestFilesJournalsSettingGroups(t *testing.T) {
	ied := startIED(t)
	e := newEngine(t)
	ctx := t.Context()
	s, err := e.OpenSession(ctx, target(t, ied.Addr))
	if err != nil {
		t.Fatal(err)
	}
	files, err := e.Files(ctx, s.ID, "COMTRADE/")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[0].Name != "COMTRADE/fault1.cfg" || files[0].Size == 0 || files[0].LastModified.IsZero() {
		t.Errorf("files = %+v", files)
	}
	if _, err := e.Files(ctx, s.ID, "*"); service.CodeOf(err) != service.CodeInvalidArgument {
		t.Errorf("wildcard = %v", err)
	}
	f, err := e.ReadFile(ctx, s.ID, "demo.icd")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(f.Data, simied.ICD()) || f.Size != int64(len(f.Data)) {
		t.Errorf("download = %d bytes, size %d", len(f.Data), f.Size)
	}
	if _, err := e.ReadFile(ctx, s.ID, "missing.txt"); service.CodeOf(err) != service.CodeIEDError {
		t.Errorf("missing file = %v", err)
	}
	if _, err := e.ReadFile(ctx, s.ID, " "); service.CodeOf(err) != service.CodeInvalidArgument {
		t.Errorf("empty name = %v", err)
	}

	journals, err := e.Journals(ctx, s.ID, simied.Domain)
	if err != nil || len(journals) != 1 {
		t.Fatalf("journals = %v, %v", journals, err)
	}
	j, err := e.ReadJournal(ctx, s.ID, model.JournalRequest{LD: simied.Domain, Journal: journals[0]})
	if err != nil {
		t.Fatal(err)
	}
	if len(j.Entries) == 0 || j.Entries[0].EntryID == "" || j.Entries[0].Time.IsZero() || len(j.Entries[0].Variables) == 0 {
		t.Errorf("journal = %+v", j)
	}
	for name, req := range map[string]model.JournalRequest{
		"no journal": {LD: simied.Domain},
		"backwards":  {LD: simied.Domain, Journal: "x", Start: time.Now(), Stop: time.Now().Add(-time.Hour)},
		"unknown LD": {LD: "x", Journal: "x"},
	} {
		if _, err := e.ReadJournal(ctx, s.ID, req); service.CodeOf(err) == "" {
			t.Errorf("%s accepted", name)
		}
	}

	sg, err := e.SettingGroups(ctx, s.ID, simied.Domain)
	if err != nil || sg.NumOfSGs != 2 || sg.ActSG != 1 {
		t.Errorf("setting groups = %+v, %v", sg, err)
	}

	// All of it is reads on the wire.
	for svc, n := range ied.Services() {
		if !readonly.Allowed(svc) {
			t.Errorf("the IED received %d %s requests", n, svc)
		}
	}
	if ied.Services()[mms.ServiceFileRead] == 0 || ied.Services()[mms.ServiceReadJournal] == 0 {
		t.Errorf("services = %v", ied.Services())
	}
}

func TestDataSetRefs(t *testing.T) {
	if got := dataSetRef("LD0", "LLN0$Meas"); got != "LD0/LLN0.Meas" {
		t.Errorf("dataSetRef = %q", got)
	}
	ld, item, err := splitDataSetRef("LD0/LLN0.Meas")
	if err != nil || ld != "LD0" || item != "LLN0$Meas" {
		t.Errorf("split = %q %q %v", ld, item, err)
	}
	for _, bad := range []string{"", "LD0", "LD0/LLN0", "LD0/LLN0.M[MX]", "/LLN0.M"} {
		if _, _, err := splitDataSetRef(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if got := flagList("none"); len(got) != 0 {
		t.Errorf("none = %v", got)
	}
}
