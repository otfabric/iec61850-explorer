// SPDX-License-Identifier: MIT

package model

import "time"

// Sources of inspected definitions.
const (
	// SourceSCL marks definitions taken from an SCL file.
	SourceSCL = "scl"
)

// DataSet is a data set (MMS named variable list) and its members.
type DataSet struct {
	// Ref is the object reference of the data set, such as
	// "IED1LD0/LLN0.Meas".
	Ref string `json:"ref"`
	// MMS is the list's name on the wire (item ID "LLN0$Meas").
	MMS MMSName `json:"mms"`
	// Deletable is the server's answer; false from SCL.
	Deletable bool   `json:"deletable"`
	Desc      string `json:"desc,omitempty"`
	// Source is "ied" or "scl".
	Source  string          `json:"source"`
	Members []DataSetMember `json:"members"`
}

// DataSetMember is one member, as an FC-qualified reference.
type DataSetMember struct {
	Ref string  `json:"ref"`
	MMS MMSName `json:"mms"`
}

// RCB is a report control block: its configuration as the IED reports it,
// or as an SCL file defines it.
type RCB struct {
	Ref      string   `json:"ref"`
	MMS      MMSName  `json:"mms"`
	Buffered bool     `json:"buffered"`
	Source   string   `json:"source"`
	RptID    string   `json:"rpt_id"`
	DatSet   string   `json:"dat_set"`
	ConfRev  uint32   `json:"conf_rev"`
	OptFlds  []string `json:"opt_flds"`
	TrgOps   []string `json:"trg_ops"`
	BufTmMS  uint32   `json:"buf_tm_ms"`
	IntgPdMS uint32   `json:"intg_pd_ms"`
	// The attributes below exist on the IED only (nil from SCL).
	RptEna  *bool   `json:"rpt_ena,omitempty"`
	Resv    *bool   `json:"resv,omitempty"`
	ResvTms *int32  `json:"resv_tms,omitempty"`
	SqNum   *uint32 `json:"sq_num,omitempty"`
	GI      *bool   `json:"gi,omitempty"`
	// EntryID and Owner are hexadecimal.
	EntryID string `json:"entry_id,omitempty"`
	Owner   string `json:"owner,omitempty"`
	// Error is set when the RCB was listed but could not be read.
	Error *OpError `json:"error,omitempty"`
}

// FileEntry is a file in the IED's file store.
type FileEntry struct {
	Name         string    `json:"name"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"last_modified,omitempty"`
}

// MaxFileBytes bounds a file downloaded from an IED.
const MaxFileBytes = 64 << 20

// FileContent is a downloaded file.
type FileContent struct {
	Name         string    `json:"name"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"last_modified,omitempty"`
	Data         []byte    `json:"data"`
}

// JournalRequest reads the entries of a journal (log) in a time range. A
// zero Stop is now; a zero Start is 24 hours before Stop.
type JournalRequest struct {
	LD      string    `json:"ld"`
	Journal string    `json:"journal"`
	Start   time.Time `json:"start,omitempty"`
	Stop    time.Time `json:"stop,omitempty"`
}

// JournalResult is one answer of the IED; MoreFollows says that it has
// more entries in the range.
type JournalResult struct {
	Entries     []JournalEntry `json:"entries"`
	MoreFollows bool           `json:"more_follows"`
}

// JournalEntry is one log entry.
type JournalEntry struct {
	// EntryID is hexadecimal.
	EntryID   string            `json:"entry_id"`
	Time      time.Time         `json:"time"`
	Variables []JournalVariable `json:"variables"`
}

// JournalVariable is one value of a log entry.
type JournalVariable struct {
	Tag   string `json:"tag"`
	Value Value  `json:"value"`
}

// SettingGroups is the setting group control block (SGCB) of a logical
// device.
type SettingGroups struct {
	LD       string `json:"ld"`
	NumOfSGs uint8  `json:"num_of_sgs"`
	ActSG    uint8  `json:"act_sg"`
	EditSG   uint8  `json:"edit_sg"`
	CnfEdit  bool   `json:"cnf_edit"`
	ResvTms  uint16 `json:"resv_tms"`
}
