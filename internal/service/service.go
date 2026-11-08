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
// In M0 every method is read-only: nothing takes a value to write, a control
// to operate or a file to delete.
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
	// Search finds nodes of the discovered model. It does not talk to the
	// IED.
	Search(ctx context.Context, id string, q model.SearchQuery) (model.SearchResult, error)
	// Read reads values. Each reference carries a functional constraint.
	// A failed reference is reported in its ReadValue; the call fails only
	// when nothing could be attempted.
	Read(ctx context.Context, id string, req model.ReadRequest) (model.ReadResult, error)
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
