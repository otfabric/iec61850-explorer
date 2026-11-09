// SPDX-License-Identifier: MIT

package model

// Semantics is what the IEC 61850 namespace (the NSD of IEC 61850-7-4 and
// the other standards in go-iec61850-model's default profile) says about a
// node: its logical node class, data object, common data class (CDC),
// attribute type and presence condition, and the engineering meaning of a
// measurement. The embedded namespaces carry identifiers and structure, no
// documentation texts.
type Semantics struct {
	Ref  string `json:"ref"`
	Kind string `json:"kind"`
	// LNClass is the logical node class; Known says the namespace defines
	// it. Namespace names the release that defines the class, such as
	// "IEC 61850-7-4:2007B6"; Base is the class it inherits from.
	LNClass    string `json:"ln_class,omitempty"`
	Known      bool   `json:"known"`
	Namespace  string `json:"namespace,omitempty"`
	Base       string `json:"base,omitempty"`
	Deprecated bool   `json:"deprecated,omitempty"`
	// DataObject is the namespace's name of the data object, without
	// instance number ("Ind" for "Ind3"); DeclaredIn is the class that
	// declares it (a base class for Beh). CDC, Presence (M, O, Omulti,
	// MOcond, ...) and the CDC the instance declares (SCL only).
	DataObject  string `json:"data_object,omitempty"`
	DeclaredIn  string `json:"declared_in,omitempty"`
	CDC         string `json:"cdc,omitempty"`
	Presence    string `json:"presence,omitempty"`
	InstanceCDC string `json:"instance_cdc,omitempty"`
	// Attribute is the attribute path below the data object, Type its
	// basic, constructed or enumeration type, FC its functional
	// constraint.
	Attribute string `json:"attribute,omitempty"`
	Type      string `json:"type,omitempty"`
	FC        string `json:"fc,omitempty"`
	// Quantity, Phase and Unit describe a measurement (go-iec61850-model
	// semantic rules), such as voltage, phase A, V.
	Quantity string `json:"quantity,omitempty"`
	Phase    string `json:"phase,omitempty"`
	Unit     string `json:"unit,omitempty"`
	// DataObjects lists, for a logical node, the class's data objects and
	// whether the node has them.
	DataObjects []NSDDataObject `json:"data_objects,omitempty"`
	// Note says why something is not known.
	Note string `json:"note,omitempty"`
}

// NSDDataObject is one data object of a logical node class.
type NSDDataObject struct {
	Name     string `json:"name"`
	CDC      string `json:"cdc"`
	Presence string `json:"presence"`
	// Mandatory is presence M.
	Mandatory bool `json:"mandatory"`
	// Present counts the node's instances of it (Ind1, Ind2: 2).
	Present int `json:"present"`
}

// Conformance finding codes.
const (
	FindingUnknownClass      = "unknown-ln-class"
	FindingMissingMandatory  = "missing-mandatory-do"
	FindingUnknownDataObject = "unknown-do"
	FindingCDCMismatch       = "cdc-mismatch"
)

// Finding severities.
const (
	SeverityError   = "error"
	SeverityWarning = "warning"
	SeverityInfo    = "info"
)

// Conformance checks a session's model against the namespace.
type Conformance struct {
	Namespaces []string  `json:"namespaces"`
	Findings   []Finding `json:"findings"`
	// LogicalNodes and DataObjects count what was checked.
	LogicalNodes int   `json:"logical_nodes"`
	DataObjects  int   `json:"data_objects"`
	Truncated    bool  `json:"truncated"`
	DurationMS   int64 `json:"duration_ms"`
}

// Finding is one conformance finding, with the evidence.
type Finding struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Ref      string `json:"ref"`
	Message  string `json:"message"`
}

// MaxFindings bounds the findings of one check.
const MaxFindings = 5000

// Point-list export formats (M3d).
const (
	ExportPointsCSV  = "points-csv"
	ExportPointsJSON = "points-json"
)

// Point is one row of a point list: a basic attribute with its functional
// constraint, what the namespace says about it, and the data sets and
// report control blocks that carry it.
type Point struct {
	Ref        string   `json:"ref"`
	LD         string   `json:"ld"`
	LN         string   `json:"ln"`
	LNClass    string   `json:"ln_class"`
	DataObject string   `json:"data_object"`
	Attribute  string   `json:"attribute"`
	FC         string   `json:"fc"`
	CDC        string   `json:"cdc,omitempty"`
	Type       string   `json:"type,omitempty"`
	Presence   string   `json:"presence,omitempty"`
	Quantity   string   `json:"quantity,omitempty"`
	Unit       string   `json:"unit,omitempty"`
	DataSets   []string `json:"data_sets"`
	Reports    []string `json:"reports"`
}

// PointList is the JSON form of a point-list export.
type PointList struct {
	Schema string  `json:"schema"`
	App    AppInfo `json:"app"`
	Source string  `json:"source"`
	Points []Point `json:"points"`
	// Note says what could not be included, such as data sets a snapshot
	// does not hold.
	Note string `json:"note,omitempty"`
}

// PointListSchema identifies the JSON point list.
const PointListSchema = "otfabric.iec61850-explorer.points/v1"
