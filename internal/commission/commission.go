// SPDX-License-Identifier: MIT

// Package commission holds the allow-lists of Commission mode (ADR 0004).
// A grant opens a second association whose go-mms DialOptions.AllowedServices
// is Services(capabilities): the Observe services of internal/readonly plus
// the state-changing services the granted capabilities need, and no other.
// The Observe association of a session is never widened.
package commission

import (
	"slices"

	"github.com/otfabric/go-mms"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/readonly"
)

// needs maps a grantable capability to the state-changing services it uses.
// Report control blocks and setting group control blocks are changed by
// writing their attributes; data sets are named variable lists.
var needs = map[string][]mms.Service{
	model.CapWrite:         {mms.ServiceWrite},
	model.CapReports:       {mms.ServiceWrite},
	model.CapDataSets:      {mms.ServiceDefineNamedVariableList, mms.ServiceDeleteNamedVariableList},
	model.CapSettingGroups: {mms.ServiceWrite},
	// Operate, SBOw and Cancel are writes; the select of a control with
	// normal security is a Read of SBO, already an Observe service, which
	// the engine sends only on the Commission association.
	model.CapControls: {mms.ServiceWrite},
}

// Grantable reports whether c is a capability a grant may carry.
func Grantable(c string) bool {
	_, ok := needs[c]
	return ok
}

// Capabilities returns the grantable capabilities, sorted.
func Capabilities() []string {
	out := make([]string, 0, len(needs))
	for c := range needs {
		out = append(out, c)
	}
	slices.Sort(out)
	return out
}

// Services returns the allow-list of a Commission association that carries
// caps. Unknown capabilities add nothing.
func Services(caps []string) []mms.Service {
	out := readonly.AllowedServices()
	for _, c := range caps {
		for _, s := range needs[c] {
			if !slices.Contains(out, s) {
				out = append(out, s)
			}
		}
	}
	return out
}
