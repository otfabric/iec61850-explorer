// SPDX-License-Identifier: MIT

package engine

import (
	iec61850 "github.com/otfabric/go-iec61850"
)

// fcControl is the functional constraint of the control services
// (IEC 61850-8-1): Oper, SBOw and Cancel are written to operate a control,
// and reading SBO *is* the select of a control with normal security. A
// read of FC CO can therefore change an IED although go-mms only sees the
// Read service that Observe mode allows, so the engine never reads it:
// not the attribute, not a data object or logical node with FC CO, and not
// a data set with such a member.
const fcControl = "CO"

// controlReadMessage explains why a CO reference is not read.
const controlReadMessage = "control attributes (FC CO) are not read: reading SBO selects a control"

func isControlFC(ref iec61850.Ref) bool { return string(ref.FC) == fcControl }
