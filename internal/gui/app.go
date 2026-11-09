// SPDX-License-Identifier: MIT

// Package gui binds the in-process engine to the Wails window: the frontend
// calls the methods of App, and App forwards the engine's events to the
// window.
//
// The desktop application is Local only. Remote use is the daemon with a
// browser; nothing here talks to a daemon or holds a token.
package gui

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
)

// EventName is the name of the Wails event that carries a model.Event.
const EventName = "explorer:event"

// App is the Wails service of the Explorer. Its exported methods are the
// frontend's whole view of the backend.
type App struct {
	svc service.Service

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// New creates the Wails service for svc.
func New(svc service.Service) *App {
	return &App{svc: svc}
}

// MarshalError encodes the error of a bound method for the frontend as
// {"code","message","detail"}.
func MarshalError(err error) []byte {
	var se *service.Error
	if !errors.As(err, &se) {
		se = &service.Error{Code: service.CodeInternal, Message: err.Error()}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			se.Code = service.CodeCanceled
		}
	}
	b, merr := json.Marshal(se)
	if merr != nil {
		return nil
	}
	return b
}

// ServiceStartup starts forwarding events to the window.
func (a *App) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	ctx, cancel := context.WithCancel(context.Background())
	events, err := a.svc.Subscribe(ctx, 0)
	if err != nil {
		cancel()
		return err
	}
	done := make(chan struct{})
	a.mu.Lock()
	a.cancel, a.done = cancel, done
	a.mu.Unlock()
	go func() {
		defer close(done)
		for ev := range events {
			if app := application.Get(); app != nil {
				app.Event.Emit(EventName, ev)
			}
		}
	}()
	return nil
}

// ServiceShutdown stops forwarding events.
func (a *App) ServiceShutdown() error {
	a.mu.Lock()
	cancel, done := a.cancel, a.done
	a.cancel, a.done = nil, nil
	a.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
	return nil
}

// Health returns the engine's versions and uptime.
func (a *App) Health(ctx context.Context) (model.Health, error) { return a.svc.Health(ctx) }

// OpenSession connects to an IED and discovers its model.
func (a *App) OpenSession(ctx context.Context, cfg model.TargetConfig) (model.SessionInfo, error) {
	return a.svc.OpenSession(ctx, cfg)
}

// CloseSession closes a session.
func (a *App) CloseSession(ctx context.Context, id string) error {
	return a.svc.CloseSession(ctx, id)
}

// Sessions lists the sessions, open and lost.
func (a *App) Sessions(ctx context.Context) ([]model.SessionInfo, error) {
	return a.svc.Sessions(ctx)
}

// Browse lists the children of a node of a session's discovered model.
func (a *App) Browse(ctx context.Context, id, ref string) ([]model.Node, error) {
	return a.svc.Browse(ctx, id, ref)
}

// Read reads values; every reference carries a functional constraint.
func (a *App) Read(ctx context.Context, id string, req model.ReadRequest) (model.ReadResult, error) {
	return a.svc.Read(ctx, id, req)
}

// StartPolling polls references of a session.
func (a *App) StartPolling(ctx context.Context, id string, cfg model.PollConfig) error {
	return a.svc.StartPolling(ctx, id, cfg)
}

// StopPolling stops polling a session.
func (a *App) StopPolling(ctx context.Context, id string) error { return a.svc.StopPolling(ctx, id) }

// Search finds nodes of a session's discovered model.
func (a *App) Search(ctx context.Context, id string, q model.SearchQuery) (model.SearchResult, error) {
	return a.svc.Search(ctx, id, q)
}

// Export reads references once and renders the session; the frontend saves
// the text with SaveFile.
func (a *App) Export(ctx context.Context, id string, req model.ExportRequest) (model.ExportFile, error) {
	return a.svc.Export(ctx, id, req)
}

// OpenSnapshot opens an offline session from a snapshot the user chose.
func (a *App) OpenSnapshot(ctx context.Context, f model.File) (model.SessionInfo, error) {
	return a.svc.OpenSnapshot(ctx, f)
}

// DataSets lists the data sets of a logical device.
func (a *App) DataSets(ctx context.Context, id, ld string) ([]model.DataSet, error) {
	return a.svc.DataSets(ctx, id, ld)
}

// Compare compares two open sessions.
func (a *App) Compare(ctx context.Context, req model.CompareRequest) (model.Comparison, error) {
	return a.svc.Compare(ctx, req)
}

// Semantics says what the IEC 61850 namespace defines for a node.
func (a *App) Semantics(ctx context.Context, id, ref string) (model.Semantics, error) {
	return a.svc.Semantics(ctx, id, ref)
}

// Conformance checks a session's model against the namespace.
func (a *App) Conformance(ctx context.Context, id string) (model.Conformance, error) {
	return a.svc.Conformance(ctx, id)
}

// Namespaces lists the embedded and loaded namespace definitions.
func (a *App) Namespaces(ctx context.Context) (model.Namespaces, error) {
	return a.svc.Namespaces(ctx)
}

// AddNamespaces loads a package of namespace definitions the user chose.
func (a *App) AddNamespaces(ctx context.Context, f model.File) (model.NamespacePackage, error) {
	return a.svc.AddNamespaces(ctx, f)
}

// RemoveNamespaces removes a loaded package of namespace definitions.
func (a *App) RemoveNamespaces(ctx context.Context, id string) error {
	return a.svc.RemoveNamespaces(ctx, id)
}

// ReadDataSet reads all members of a data set.
func (a *App) ReadDataSet(ctx context.Context, id, ref string) (model.ReadResult, error) {
	return a.svc.ReadDataSet(ctx, id, ref)
}

// Reports lists the report control blocks of a logical device.
func (a *App) Reports(ctx context.Context, id, ld string) ([]model.RCB, error) {
	return a.svc.Reports(ctx, id, ld)
}

// Files lists the IED's files under a directory.
func (a *App) Files(ctx context.Context, id, dir string) ([]model.FileEntry, error) {
	return a.svc.Files(ctx, id, dir)
}

// Journals lists the journals of a logical device.
func (a *App) Journals(ctx context.Context, id, ld string) ([]string, error) {
	return a.svc.Journals(ctx, id, ld)
}

// ReadJournal reads journal entries in a time range.
func (a *App) ReadJournal(ctx context.Context, id string, req model.JournalRequest) (model.JournalResult, error) {
	return a.svc.ReadJournal(ctx, id, req)
}

// SettingGroups reads the setting group control block of a logical device.
func (a *App) SettingGroups(ctx context.Context, id, ld string) (model.SettingGroups, error) {
	return a.svc.SettingGroups(ctx, id, ld)
}

// GrantCommission puts a session in Commission mode.
func (a *App) GrantCommission(ctx context.Context, id string, req model.GrantRequest) (model.SessionInfo, error) {
	return a.svc.GrantCommission(ctx, id, req)
}

// RevokeCommission ends a session's Commission grant.
func (a *App) RevokeCommission(ctx context.Context, id string) (model.SessionInfo, error) {
	return a.svc.RevokeCommission(ctx, id)
}

// Write writes a data attribute and reads it back.
func (a *App) Write(ctx context.Context, id string, req model.WriteRequest) (model.WriteResult, error) {
	return a.svc.Write(ctx, id, req)
}

// Control operates a controllable data object.
func (a *App) Control(ctx context.Context, id string, req model.ControlRequest) (model.ControlResult, error) {
	return a.svc.Control(ctx, id, req)
}

// SubscribeReport subscribes to a report control block.
func (a *App) SubscribeReport(ctx context.Context, id string, req model.SubscribeRequest) (model.Subscription, error) {
	return a.svc.SubscribeReport(ctx, id, req)
}

// Unsubscribe ends a report subscription.
func (a *App) Unsubscribe(ctx context.Context, id, subscriptionID string) error {
	return a.svc.Unsubscribe(ctx, id, subscriptionID)
}

// TriggerGI asks for a general interrogation on a subscription.
func (a *App) TriggerGI(ctx context.Context, id, subscriptionID string) error {
	return a.svc.TriggerGI(ctx, id, subscriptionID)
}

// CreateDataSet defines a data set on the IED.
func (a *App) CreateDataSet(ctx context.Context, id string, req model.CreateDataSetRequest) (model.DataSet, error) {
	return a.svc.CreateDataSet(ctx, id, req)
}

// DeleteDataSet deletes a deletable data set.
func (a *App) DeleteDataSet(ctx context.Context, id, ref string) error {
	return a.svc.DeleteDataSet(ctx, id, ref)
}

// SelectActiveSG selects the active setting group.
func (a *App) SelectActiveSG(ctx context.Context, id string, req model.SelectSGRequest) (model.SettingGroups, error) {
	return a.svc.SelectActiveSG(ctx, id, req)
}

// Audit returns recent audit records.
func (a *App) Audit(ctx context.Context, q model.AuditQuery) ([]model.AuditRecord, error) {
	return a.svc.Audit(ctx, q)
}

// SaveDeviceFile downloads a file of the IED and asks the user where to save
// it. It returns the chosen path, or "" when the user cancelled. The frontend
// names the IED's file; the local path comes from the native dialog.
func (a *App) SaveDeviceFile(ctx context.Context, id, name string) (string, error) {
	f, err := a.svc.ReadFile(ctx, id, name)
	if err != nil {
		return "", err
	}
	app := application.Get()
	if app == nil {
		return "", service.Errorf(service.CodeInternal, "no application")
	}
	path, err := app.Dialog.SaveFile().SetFilename(filepath.Base(filepath.ToSlash(f.Name))).CanCreateDirectories(true).PromptForSingleSelection()
	if err != nil {
		return "", service.Wrap(service.CodeInternal, err, "save dialog")
	}
	if path == "" {
		return "", nil
	}
	if err := os.WriteFile(path, f.Data, 0o644); err != nil {
		return "", service.Wrap(service.CodeInternal, err, "cannot save the file")
	}
	return path, nil
}

// InspectSCL parses an SCL document the user chose in the window.
func (a *App) InspectSCL(ctx context.Context, f model.File) (model.SCLInfo, error) {
	return a.svc.InspectSCL(ctx, f)
}

// OpenSCL opens an offline session on an IED of an SCL document.
func (a *App) OpenSCL(ctx context.Context, req model.OpenSCLRequest) (model.SessionInfo, error) {
	return a.svc.OpenSCL(ctx, req)
}

// Profiles lists the saved connection profiles.
func (a *App) Profiles(ctx context.Context) ([]model.Profile, error) { return a.svc.Profiles(ctx) }

// SaveProfile creates or replaces a connection profile.
func (a *App) SaveProfile(ctx context.Context, p model.Profile) (model.Profile, error) {
	return a.svc.SaveProfile(ctx, p)
}

// DeleteProfile removes a connection profile.
func (a *App) DeleteProfile(ctx context.Context, id string) error {
	return a.svc.DeleteProfile(ctx, id)
}

// SaveFile asks the user where to save text under the suggested name and
// writes it there. It returns the chosen path, or "" when the user cancelled.
// The path comes from the native dialog, never from the frontend.
func (a *App) SaveFile(name, _ string, text string) (string, error) {
	app := application.Get()
	if app == nil {
		return "", service.Errorf(service.CodeInternal, "no application")
	}
	name = filepath.Base(name)
	dialog := app.Dialog.SaveFile().SetFilename(name).CanCreateDirectories(true)
	if ext := strings.TrimPrefix(filepath.Ext(name), "."); ext != "" {
		dialog.AddFilter(strings.ToUpper(ext)+" files", "*."+ext)
	}
	path, err := dialog.PromptForSingleSelection()
	if err != nil {
		return "", service.Wrap(service.CodeInternal, err, "save dialog")
	}
	if path == "" {
		return "", nil
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return "", service.Wrap(service.CodeInternal, err, "cannot save the file")
	}
	return path, nil
}

// OpenExternal opens an https link in the user's browser.
func (a *App) OpenExternal(link string) error {
	u, err := url.Parse(link)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return service.Errorf(service.CodeInvalidArgument, "only https links can be opened")
	}
	app := application.Get()
	if app == nil {
		return service.Errorf(service.CodeInternal, "no application")
	}
	return app.Browser.OpenURL(u.String())
}
