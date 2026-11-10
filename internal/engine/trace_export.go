// SPDX-License-Identifier: MIT

package engine

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/otfabric/iec61850-explorer/internal/buildinfo"
	"github.com/otfabric/iec61850-explorer/internal/model"
)

// TraceExportSchema identifies the JSON exports of a trace session.
const TraceExportSchema = "otfabric.iec61850-explorer.trace/1"

// traceExport is the JSON document of a trace export.
type traceExport struct {
	Schema       string                   `json:"schema"`
	App          model.AppInfo            `json:"app"`
	Engine       string                   `json:"engine"`
	ExportedAt   time.Time                `json:"exported_at"`
	File         model.TraceFile          `json:"file"`
	Link         *model.TraceLink         `json:"link,omitempty"`
	Findings     []model.TraceFinding     `json:"findings,omitempty"`
	Transactions []model.TraceTransaction `json:"transactions,omitempty"`
}

func timeText(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339Nano)
}

// exportTrace renders a trace session's findings or timeline as CSV or
// JSON, whole. CSV cells are protected against formula injection.
func exportTrace(ts *traceState, format string) (model.ExportFile, error) {
	link := ts.currentLink()
	base := strings.TrimSuffix(strings.TrimSuffix(ts.file.Name, ".gz"), ".pcapng")
	base = strings.TrimSuffix(strings.TrimSuffix(base, ".pcap"), ".cap")
	base = downloadBase(base)
	doc := traceExport{
		Schema: TraceExportSchema, App: appInfo(), Engine: "go-mms-trace " + buildinfo.ModuleVersion(buildinfo.GoMMSTracePath),
		ExportedAt: time.Now().UTC(), File: ts.file, Link: ts.linkInfo(),
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	switch format {
	case model.ExportFindingsJSON, model.ExportFindingsCSV:
		doc.Findings = ts.allFindings()
		if format == model.ExportFindingsJSON {
			break
		}
		_ = w.Write([]string{"severity", "kind", "association", "frame", "time", "transaction", "stream", "source", "count", "message"})
		for _, f := range doc.Findings {
			_ = w.Write(csvRow([]string{f.Severity, f.Kind, intText(f.Association), uintText(f.Frame), timeText(f.At),
				intText(f.Transaction), f.Stream, f.Source, uintText(f.Count), f.Message}))
		}
	default:
		doc.Transactions = make([]model.TraceTransaction, 0, len(ts.res.Transactions))
		for i, t := range ts.res.Transactions {
			doc.Transactions = append(doc.Transactions, transactionOf(i+1, t, link))
		}
		if format == model.ExportTimelineJSON {
			break
		}
		_ = w.Write([]string{"index", "association", "invoke_id", "service", "status", "request_time", "response_time", "latency_ms",
			"request_frame", "response_frame", "objects", "mms_names", "model", "error"})
		for _, t := range doc.Transactions {
			var refs, names, res []string
			for _, o := range t.Objects {
				refs, names = append(refs, o.Ref), append(names, o.MMS.Domain+"/"+o.MMS.ItemID)
				res = append(res, o.Model)
			}
			latency := ""
			if t.LatencyMS != nil {
				latency = strconv.FormatFloat(*t.LatencyMS, 'f', 3, 64)
			}
			_ = w.Write(csvRow([]string{strconv.Itoa(t.Index), strconv.Itoa(t.Association), strconv.FormatUint(uint64(t.InvokeID), 10),
				t.Service, t.Status, timeText(t.RequestAt), timeText(t.ResponseAt), latency, uintText(t.RequestFrame),
				uintText(t.ResponseFrame), strings.Join(refs, ";"), strings.Join(names, ";"), strings.Join(res, ";"), t.Error}))
		}
	}
	what := "-findings"
	if format == model.ExportTimelineCSV || format == model.ExportTimelineJSON {
		what = "-timeline"
	}
	if format == model.ExportFindingsJSON || format == model.ExportTimelineJSON {
		b, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return model.ExportFile{}, err
		}
		return model.ExportFile{Name: base + what + ".json", MIME: "application/json", Text: string(b) + "\n"}, nil
	}
	w.Flush()
	return model.ExportFile{Name: base + what + ".csv", MIME: "text/csv", Text: buf.String()}, nil
}

func intText(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n)
}

func uintText(n uint64) string {
	if n == 0 {
		return ""
	}
	return strconv.FormatUint(n, 10)
}

// downloadBase keeps letters, digits, dot, dash and underscore of a name.
func downloadBase(name string) string {
	out := []rune{}
	for _, c := range name {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '.', c == '-', c == '_':
			out = append(out, c)
		default:
			out = append(out, '_')
		}
	}
	if s := strings.Trim(string(out), "."); s != "" {
		return s
	}
	return "trace"
}
