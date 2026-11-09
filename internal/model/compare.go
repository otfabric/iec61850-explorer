// SPDX-License-Identifier: MIT

package model

// CompareRequest compares two open sessions: A is the reference (an SCL
// file, say) and B the subject (the IED, say). Any two of a live session,
// an SCL session and a snapshot can be compared.
type CompareRequest struct {
	A string `json:"a"`
	B string `json:"b"`
}

// Comparison categories.
const (
	CompareModel    = "model"
	CompareDataSets = "datasets"
	CompareReports  = "reports"
)

// Difference kinds.
const (
	DiffOnlyA   = "only_a"
	DiffOnlyB   = "only_b"
	DiffChanged = "changed"
)

// MaxDifferences bounds the differences a comparison returns; Truncated
// says when there were more.
const MaxDifferences = 5000

// Comparison is the result of comparing two sessions. Every difference
// names the reference and, for a changed item, the field and both values,
// so that it can be checked on either side.
type Comparison struct {
	A CompareSide `json:"a"`
	B CompareSide `json:"b"`
	// Categories says, per category, whether it was compared and what was
	// found; a category that could not be compared gives the reason.
	Categories  []CompareCategory `json:"categories"`
	Differences []Difference      `json:"differences"`
	Truncated   bool              `json:"truncated"`
	DurationMS  int64             `json:"duration_ms"`
}

// CompareSide describes one compared session.
type CompareSide struct {
	SessionID string `json:"session_id"`
	// Source is "ied", "scl" or "snapshot".
	Source string `json:"source"`
	Label  string `json:"label"`
}

// CompareCategory summarises one category.
type CompareCategory struct {
	Name     string `json:"name"`
	Compared bool   `json:"compared"`
	// Reason is set when the category was not compared, or compared only
	// in part (logical devices one side could not list).
	Reason string `json:"reason,omitempty"`
	// Items is the number of items present on both sides; Same of them are
	// equal in every compared field.
	Items   int `json:"items"`
	Same    int `json:"same"`
	OnlyA   int `json:"only_a"`
	OnlyB   int `json:"only_b"`
	Changed int `json:"changed"`
}

// Difference is one mismatch. A node missing on one side is reported once:
// the nodes below it are not listed.
type Difference struct {
	Category string `json:"category"`
	Kind     string `json:"kind"`
	// Ref is the object reference of the node, data set or report control
	// block (without functional constraint).
	Ref string `json:"ref"`
	// Field is the compared field of a changed item, such as "fcs",
	// "members" or "conf_rev".
	Field string `json:"field,omitempty"`
	A     string `json:"a,omitempty"`
	B     string `json:"b,omitempty"`
	// Note explains a difference whose values alone do not, such as the
	// first differing member of a data set.
	Note string `json:"note,omitempty"`
}
