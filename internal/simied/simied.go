// SPDX-License-Identifier: MIT

// Package simied serves a simulated IED for the Explorer's tests: the model
// of demo.icd on go-iec61850's server, on a loopback port. It counts every
// MMS service the server receives, which is how the wire tests prove that the
// Explorer sends nothing but reads.
//
// Only _test.go files may import it (checked by TestSimIEDOnlyInTests in
// internal/readonly): the programs never ship a server.
package simied

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"io/fs"
	"maps"
	"sort"
	"strings"
	"sync"
	"time"

	iec61850 "github.com/otfabric/go-iec61850"
	"github.com/otfabric/go-iec61850/scl"
	"github.com/otfabric/go-mms"
	"github.com/otfabric/go-mms/transport/iso"
)

//go:embed demo.icd
var icd []byte

// Names of the simulated IED.
const (
	IEDName = "DemoIED"
	// Domain is the MMS domain of logical device LD0 (IED name + inst).
	Domain = IEDName + "LD0"
)

// Process values the IED starts with.
const (
	Frequency = 50.002
	TotW      = 1245000.0
)

// IED is a running simulated IED.
type IED struct {
	// Addr is host:port of the MMS listener.
	Addr string

	srv    *iec61850.Server
	cancel context.CancelFunc
	done   chan struct{}
	m      *serviceCounter
	once   sync.Once
}

// Start serves the simulated IED on 127.0.0.1 until Close.
func Start() (*IED, error) {
	doc, err := scl.Parse(bytes.NewReader(icd))
	if err != nil {
		return nil, fmt.Errorf("simied: SCL: %w", err)
	}
	model, err := iec61850.NewServerModelFromSCL(doc, IEDName, "")
	if err != nil {
		return nil, fmt.Errorf("simied: model: %w", err)
	}
	counter := &serviceCounter{counts: make(map[mms.Service]int)}
	srv, err := iec61850.NewServer(model, iec61850.ServerOptions{
		Identity:     &iec61850.ServerIdentity{Vendor: "OT Fabric", Model: "Explorer demo IED", Revision: "1"},
		FileProvider: Files,
		MMS:          mms.ServerOptions{Metrics: counter, JournalProvider: iec61850.NewMemoryJournalProvider()},
	})
	if err != nil {
		return nil, fmt.Errorf("simied: server: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	now := time.Date(2026, 11, 8, 14, 32, 8, 417_000_000, time.UTC)
	good := iec61850.EncodeQuality(0)
	ts := iec61850.EncodeTimestamp(iec61850.Timestamp{Time: now})
	set := func(item string, v *mms.Value) { srv.SetValue(ctx, Domain+"/"+item, v) }
	set("MMXU1$MX$Hz$mag$f", mms.NewFloat(Frequency))
	set("MMXU1$MX$Hz$q", good)
	set("MMXU1$MX$Hz$t", ts)
	set("MMXU1$MX$TotW$mag$f", mms.NewFloat(TotW))
	set("MMXU1$MX$TotW$q", iec61850.EncodeQuality(iec61850.Quality(iec61850.ValidityQuestionable)|iec61850.QualityOldData))
	set("MMXU1$MX$TotW$t", ts)
	set("XCBR1$ST$Pos$stVal", mms.NewBitStringWithLength([]byte{0x80}, 2)) // on (10)
	set("XCBR1$ST$Pos$q", good)
	set("XCBR1$ST$Pos$t", ts)
	set("XCBR1$ST$OpCnt$stVal", mms.NewInteger(17))
	set("GGIO1$ST$Ind1$stVal", mms.NewBoolean(true))
	set("LLN0$ST$Mod$stVal", mms.NewInteger(1))
	set("LLN0$ST$Beh$stVal", mms.NewInteger(1))
	set("LLN0$ST$Health$stVal", mms.NewInteger(1))
	srv.EnableReports()
	srv.EnableJournals()
	srv.EnableSettingGroups(iec61850.SettingGroupHandler{})
	// Two changes of the frequency: the log holds them.
	set("MMXU1$MX$Hz$mag$f", mms.NewFloat(49.998))
	set("MMXU1$MX$Hz$mag$f", mms.NewFloat(Frequency))

	ln, err := iso.Listen("127.0.0.1:0")
	if err != nil {
		cancel()
		srv.Close()
		return nil, fmt.Errorf("simied: listen: %w", err)
	}
	ied := &IED{Addr: ln.Addr().String(), srv: srv, cancel: cancel, done: make(chan struct{}), m: counter}
	go func() {
		defer close(ied.done)
		// How the accept loop ended is nothing to report to a test.
		_ = srv.ListenAndServe(ctx, ln)
	}()
	return ied, nil
}

// Close stops the IED. It may be called more than once.
func (d *IED) Close() {
	d.once.Do(func() {
		d.cancel()
		_ = d.srv.Shutdown(context.Background())
		<-d.done // the accept loop has ended
		d.srv.Close()
	})
}

// Set changes a value of the IED, as its process would: item is an MMS
// item ID of the logical device, such as "MMXU1$MX$Hz$mag$f". A change
// triggers the reports and logs that watch it.
func (d *IED) Set(item string, v *mms.Value) {
	d.srv.SetValue(context.Background(), Domain+"/"+item, v)
}

// Services returns how often the server received each confirmed service.
func (d *IED) Services() map[mms.Service]int {
	return d.m.snapshot()
}

// Connections returns the number of associations the server saw established.
func (d *IED) Connections() int {
	d.m.mu.Lock()
	defer d.m.mu.Unlock()
	return d.m.established
}

// serviceCounter is the server's go-mms Metrics.
type serviceCounter struct {
	mms.NopMetrics
	mu          sync.Mutex
	counts      map[mms.Service]int
	established int
}

func (c *serviceCounter) ObserveRequest(ev mms.RequestEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counts[ev.Service]++
}

func (c *serviceCounter) ObserveConnection(ev mms.ConnectionEvent) {
	if ev.State != mms.ConnectionEstablished {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.established++
}

func (c *serviceCounter) snapshot() map[mms.Service]int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return maps.Clone(c.counts)
}

// ICD returns the SCL document the simulated IED serves.
func ICD() []byte { return append([]byte(nil), icd...) }

// Files is the IED's file store: a disturbance record and the ICD.
var Files = fileStore{
	"COMTRADE/fault1.cfg": []byte("Explorer demo IED,1,1999\n2,1A,1D\n"),
	"COMTRADE/fault1.dat": []byte("1,0,10,0\n2,500,12,1\n"),
	"demo.icd":            icd,
}

// fileStore is a read-only mms.FileProvider over names and contents.
type fileStore map[string][]byte

type fileHandle struct {
	data []byte
	off  int
}

var filesTime = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

// List implements mms.FileProvider: the files whose name starts with the
// FileSpec, in name order.
func (f fileStore) List(_ context.Context, req mms.FileListRequest) (*mms.FileListResult, error) {
	names := make([]string, 0, len(f))
	for name := range f {
		if strings.HasPrefix(name, req.FileSpec) && name > req.ContinueAfter {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	res := &mms.FileListResult{}
	for _, name := range names {
		if req.MaxEntries > 0 && len(res.Entries) == req.MaxEntries {
			res.MoreFollows = true
			break
		}
		res.Entries = append(res.Entries, mms.FileEntry{Name: name, Size: int64(len(f[name])), LastModified: filesTime})
	}
	return res, nil
}

// Open implements mms.FileProvider.
func (f fileStore) Open(_ context.Context, path string) (mms.FileHandle, mms.FileAttributes, error) {
	data, ok := f[path]
	if !ok {
		return nil, mms.FileAttributes{}, fs.ErrNotExist
	}
	return &fileHandle{data: data}, mms.FileAttributes{Size: int64(len(data)), LastModified: filesTime}, nil
}

// Read implements mms.FileProvider.
func (fileStore) Read(_ context.Context, h mms.FileHandle, maxBytes int) ([]byte, bool, error) {
	fh, ok := h.(*fileHandle)
	if !ok {
		return nil, false, fmt.Errorf("simied: bad file handle %T", h)
	}
	n := min(maxBytes, len(fh.data)-fh.off)
	chunk := fh.data[fh.off : fh.off+n]
	fh.off += n
	return chunk, fh.off < len(fh.data), nil
}

// Close implements mms.FileProvider.
func (fileStore) Close(context.Context, mms.FileHandle) error { return nil }

// Delete, Rename and ObtainFile are refused: the files are read-only.
func (fileStore) Delete(context.Context, string) error             { return fs.ErrPermission }
func (fileStore) Rename(context.Context, string, string) error     { return fs.ErrPermission }
func (fileStore) ObtainFile(context.Context, string, string) error { return fs.ErrPermission }
