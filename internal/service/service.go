// SPDX-License-Identifier: MIT

// Package service defines the one interface the user interfaces talk to, and
// its errors. The in-process engine implements it; the Wails binding and the
// daemon's HTTP handlers wrap it, and the contract tests run against both.
package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/otfabric/iec61850-explorer/internal/model"
)

// Service is the IEC 61850 engine as the user interfaces see it.
//
// Every method but the Commission ones (GrantCommission, RevokeCommission,
// Write) is read-only; those change the IED only through a granted,
// audited second association (ADR 0004).
type Service interface {
	// Health reports versions, mode and uptime.
	Health(ctx context.Context) (model.Health, error)
	// OpenSession connects to the IED, asks for its identity and discovers
	// its model.
	OpenSession(ctx context.Context, cfg model.TargetConfig) (model.SessionInfo, error)
	// CloseSession concludes the association.
	CloseSession(ctx context.Context, id string) error
	// Sessions lists the sessions, open and lost.
	Sessions(ctx context.Context) ([]model.SessionInfo, error)
	// Browse lists the children of a node of the discovered model: the
	// logical devices for "", the logical nodes of "LD", the data objects of
	// "LD/LN", and so on. It does not talk to the IED.
	Browse(ctx context.Context, id, ref string) ([]model.Node, error)
	// StartPolling polls references of a session, replacing a running
	// poller. Samples arrive as events.
	StartPolling(ctx context.Context, id string, cfg model.PollConfig) error
	// StopPolling stops polling; a session that does not poll is not an
	// error.
	StopPolling(ctx context.Context, id string) error
	// Search finds nodes of the discovered model. It does not talk to the
	// IED.
	Search(ctx context.Context, id string, q model.SearchQuery) (model.SearchResult, error)
	// Read reads values. Each reference carries a functional constraint.
	// A failed reference is reported in its ReadValue; the call fails only
	// when nothing could be attempted.
	Read(ctx context.Context, id string, req model.ReadRequest) (model.ReadResult, error)
	// InspectSCL parses an SCL document and describes its IEDs.
	InspectSCL(ctx context.Context, f model.File) (model.SCLInfo, error)
	// OpenSCL opens an offline session on an IED of an SCL document: its
	// model can be browsed and searched, not read.
	OpenSCL(ctx context.Context, req model.OpenSCLRequest) (model.SessionInfo, error)
	// Export reads references once and renders the session as a snapshot
	// (JSON) or CSV.
	Export(ctx context.Context, id string, req model.ExportRequest) (model.ExportFile, error)
	// OpenSnapshot opens an offline session from a snapshot; its reads
	// return the snapshot's values.
	OpenSnapshot(ctx context.Context, f model.File) (model.SessionInfo, error)
	// DataSets lists the data sets of a logical device with their members:
	// from the IED, or from the SCL file of an offline session.
	DataSets(ctx context.Context, id, ld string) ([]model.DataSet, error)
	// ReadDataSet reads all members of a data set with one request.
	ReadDataSet(ctx context.Context, id, ref string) (model.ReadResult, error)
	// Reports lists the report control blocks of a logical device with
	// their configuration.
	Reports(ctx context.Context, id, ld string) ([]model.RCB, error)
	// Files lists the IED's files under a directory ("" is the root).
	Files(ctx context.Context, id, dir string) ([]model.FileEntry, error)
	// ReadFile downloads a file, at most model.MaxFileBytes.
	ReadFile(ctx context.Context, id, name string) (model.FileContent, error)
	// Journals lists the journals (logs) of a logical device.
	Journals(ctx context.Context, id, ld string) ([]string, error)
	// ReadJournal reads journal entries in a time range.
	ReadJournal(ctx context.Context, id string, req model.JournalRequest) (model.JournalResult, error)
	// SettingGroups reads the setting group control block of a logical
	// device.
	SettingGroups(ctx context.Context, id, ld string) (model.SettingGroups, error)
	// GrantCommission puts a session in Commission mode: it opens a second
	// association whose allow-list follows the granted capabilities, and
	// records the grant in the audit trail.
	GrantCommission(ctx context.Context, id string, req model.GrantRequest) (model.SessionInfo, error)
	// RevokeCommission ends a grant: it undoes what the engine set up for
	// itself and concludes the second association. A session without a
	// grant is not an error.
	RevokeCommission(ctx context.Context, id string) (model.SessionInfo, error)
	// Write writes one data attribute and reads it back. It needs a grant
	// with the write capability. An IED that refuses the write is reported
	// in the result.
	Write(ctx context.Context, id string, req model.WriteRequest) (model.WriteResult, error)
	// SubscribeReport subscribes to a report control block: reserve
	// (unbuffered), enable, optionally GI. Reports arrive as events. Needs
	// the reports capability.
	SubscribeReport(ctx context.Context, id string, req model.SubscribeRequest) (model.Subscription, error)
	// Unsubscribe disables and releases the RCB of a subscription.
	Unsubscribe(ctx context.Context, id, subscriptionID string) error
	// TriggerGI asks for a general interrogation on a subscription.
	TriggerGI(ctx context.Context, id, subscriptionID string) error
	// CreateDataSet defines a persistent data set. Needs the datasets
	// capability.
	CreateDataSet(ctx context.Context, id string, req model.CreateDataSetRequest) (model.DataSet, error)
	// DeleteDataSet deletes a data set the IED marks deletable.
	DeleteDataSet(ctx context.Context, id, ref string) error
	// SelectActiveSG selects the active setting group and reads the setting
	// group control block back. Needs the setting-groups capability.
	SelectActiveSG(ctx context.Context, id string, req model.SelectSGRequest) (model.SettingGroups, error)
	// Audit returns recent audit records, newest first.
	Audit(ctx context.Context, q model.AuditQuery) ([]model.AuditRecord, error)
	// Profiles lists the saved connection profiles, by name.
	Profiles(ctx context.Context) ([]model.Profile, error)
	// SaveProfile creates a profile (empty ID) or replaces one.
	SaveProfile(ctx context.Context, p model.Profile) (model.Profile, error)
	// DeleteProfile removes a profile.
	DeleteProfile(ctx context.Context, id string) error
	// Subscribe returns the event stream. Events with a sequence number above
	// sinceSeq that the engine still holds are replayed first; sinceSeq 0
	// delivers new events only. The channel is closed when ctx ends.
	Subscribe(ctx context.Context, sinceSeq uint64) (<-chan model.Event, error)
}

// Error codes. They are part of the API: clients switch on them.
const (
	CodeInvalidArgument = "invalid_argument"
	CodeNotFound        = "not_found"
	CodeLimitExceeded   = "limit_exceeded"
	CodeNotAllowed      = "not_allowed"
	// CodeConnectFailed means the IED could not be reached or refused the
	// association. Detail holds the classified error.
	CodeConnectFailed = "connect_failed"
	// CodeDiscoveryFailed means the association stood but the model could
	// not be listed.
	CodeDiscoveryFailed = "discovery_failed"
	// CodeIEDError means the IED answered a request with an error, or the
	// exchange failed. Detail holds the classified error.
	CodeIEDError = "ied_error"
	// CodeSessionLost means the session's association ended.
	CodeSessionLost = "session_lost"
	// CodeUnauthorized means the request carried no valid bearer token, or a
	// Host header the daemon does not answer to.
	CodeUnauthorized = "unauthorized"
	// CodeCanceled means the caller gave up before the call finished.
	CodeCanceled = "canceled"
	CodeInternal = "internal"
)

// Error is the error type of every Service method.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	// Detail is the classified protocol error of a failed connect or
	// discovery.
	Detail *model.OpError `json:"detail,omitempty"`
	// Err is the underlying error, for errors.Is in the same process. It does
	// not cross the HTTP API.
	Err error `json:"-"`
}

// Error returns the message.
func (e *Error) Error() string { return e.Message }

// Unwrap returns the underlying error.
func (e *Error) Unwrap() error { return e.Err }

// Errorf creates an Error with a formatted message.
func Errorf(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// CodeOf returns the code of err, CodeInternal for an error that is not an
// *Error, and "" for nil.
func CodeOf(err error) string {
	if err == nil {
		return ""
	}
	if e, ok := errors.AsType[*Error](err); ok {
		return e.Code
	}
	return CodeInternal
}

// Wrap creates an Error from err with a message prefix.
func Wrap(code string, err error, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...) + ": " + err.Error(), Err: err}
}
