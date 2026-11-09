// SPDX-License-Identifier: MIT

package model

import "time"

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
	// Declared is the namespace the node's logical device or node
	// declares, and CheckedAgainst the releases its definitions come from.
	Declared       string   `json:"declared,omitempty"`
	CheckedAgainst []string `json:"checked_against,omitempty"`
	Edition1       bool     `json:"edition1,omitempty"`
	// Title and Description are the documentation texts of the class or
	// data object, from a loaded package with NSDoc; not exported.
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
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
	// Namespaces are all releases used; Declared says, per logical device
	// (and logical node that declares its own), what was declared and
	// checked.
	Namespaces []string            `json:"namespaces"`
	Declared   []DeclaredNamespace `json:"declared"`
	Findings   []Finding           `json:"findings"`
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

// NamespacePackage is a package of namespace definitions (NSD, IEC
// 61850-7-7) a user loaded: official releases the Explorer does not embed,
// or a vendor's extension namespaces. Its documentation texts, when it has
// them, are shown but never exported.
type NamespacePackage struct {
	// ID identifies the package: the start of its SHA-256.
	ID      string    `json:"id"`
	File    string    `json:"file"`
	SHA256  string    `json:"sha256"`
	AddedAt time.Time `json:"added_at"`
	// Namespaces are the releases the package defines, such as
	// "IEC 61850-7-4:2007B4" or a vendor's "ABB:2016A".
	Namespaces []string `json:"namespaces"`
	// Docs says the package has documentation texts (NSDoc).
	Docs bool `json:"docs"`
}

// Namespaces lists what the engine checks models against.
type Namespaces struct {
	// Embedded are the releases built into the Explorer
	// (go-iec61850-model), Profiles its named selections of them.
	Embedded []string           `json:"embedded"`
	Profiles []NamespaceProfile `json:"profiles"`
	Loaded   []NamespacePackage `json:"loaded"`
}

// NamespaceProfile is a named selection of embedded releases.
type NamespaceProfile struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Namespaces  []string `json:"namespaces"`
}

// MaxNamespaceBytes bounds a namespace package.
const MaxNamespaceBytes = 32 << 20

// MaxNamespacePackages bounds the loaded packages.
const MaxNamespacePackages = 64

// DeclaredNamespace is the namespace a logical device (LLN0.NamPlt.ldNs)
// or a logical node (NamPlt.lnNs, overriding it) declares, and what the
// check used for it.
type DeclaredNamespace struct {
	Ref string `json:"ref"`
	// Declared is the declaration, empty when there is none.
	Declared string `json:"declared,omitempty"`
	// Source is "ldNs", "lnNs", "scl-schema" (no declaration; the SCL
	// file's schema version) or "none".
	Source string `json:"source"`
	// CheckedAgainst lists the releases used.
	CheckedAgainst []string `json:"checked_against"`
	// Edition1 marks a device of IEC 61850-7-4 Edition 1 (2003), for which
	// IEC publishes no namespace definitions: it is checked against
	// Edition 2, and what Edition 2 added is reported as a warning.
	Edition1 bool `json:"edition1,omitempty"`
	// Unknown marks a declaration that is neither embedded nor loaded.
	Unknown bool   `json:"unknown,omitempty"`
	Note    string `json:"note,omitempty"`
}

// FindingNamespaceNotLoaded is a declared namespace that is neither
// embedded nor loaded.
const FindingNamespaceNotLoaded = "namespace-not-loaded"
