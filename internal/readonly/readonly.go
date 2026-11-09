// SPDX-License-Identifier: MIT

// Package readonly holds the Explorer's read-only guard: the MMS services an
// Observe-mode client may send. Every MMS client the engine creates sets
// go-mms's DialOptions.AllowedServices to Services, so a request for any
// other service is refused inside go-mms before a byte is sent (it fails with
// mms.ErrServiceNotAllowed). The wire tests in internal/engine check the
// services a server actually receives.
package readonly

import (
	"slices"

	"github.com/otfabric/go-mms"
)

// Services are the confirmed MMS services of Observe mode. None of them
// changes the configuration or the process state of the server.
// Association control (Initiate, Conclude, Abort) is not a confirmed service
// and is not affected.
//
// FileOpen and FileClose open and release a file read state machine (FRSM)
// on the server for one download; they are part of reading a file (ISO
// 9506-2 annex D) and change nothing the IED stores. FileRename, FileDelete
// and ObtainFile are not here.
var Services = []mms.Service{
	mms.ServiceIdentify,
	mms.ServiceStatus,
	mms.ServiceGetNameList,
	mms.ServiceRead,
	mms.ServiceGetVariableAccessAttributes,
	mms.ServiceGetNamedVariableListAttributes,
	mms.ServiceFileDirectory,
	mms.ServiceFileOpen,
	mms.ServiceFileRead,
	mms.ServiceFileClose,
	mms.ServiceReadJournal,
}

// Allowed reports whether s is one of Services.
func Allowed(s mms.Service) bool {
	return slices.Contains(Services, s)
}

// AllowedServices returns a copy of Services for DialOptions.AllowedServices,
// so that a caller cannot widen the package's list.
func AllowedServices() []mms.Service {
	return slices.Clone(Services)
}
