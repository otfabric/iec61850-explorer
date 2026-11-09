// SPDX-License-Identifier: MIT

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/proxy"
	"github.com/otfabric/iec61850-explorer/internal/service"
)

// SettingsSchema identifies the settings file.
const SettingsSchema = "otfabric.iec61850-explorer.settings/1"

// maxSettingsFile bounds the settings file the engine reads.
const maxSettingsFile = 64 << 10

type settingsFile struct {
	Schema       string `json:"schema"`
	DefaultProxy string `json:"default_proxy,omitempty"`
}

// settingsStore keeps the user's settings in <data-dir>/settings.json, mode
// 0600, written atomically like the profiles.
type settingsStore struct {
	path string // empty: kept in memory only

	mu           sync.Mutex
	defaultProxy string
}

func (st *settingsStore) load() error {
	if st.path == "" {
		return nil
	}
	f, err := os.Open(st.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Size() > maxSettingsFile {
		return fmt.Errorf("%s is larger than %d bytes", st.path, maxSettingsFile)
	}
	var file settingsFile
	if err := json.NewDecoder(f).Decode(&file); err != nil {
		return fmt.Errorf("%s: %w", st.path, err)
	}
	if file.Schema != SettingsSchema {
		return fmt.Errorf("%s: schema %q, want %q", st.path, file.Schema, SettingsSchema)
	}
	if _, err := parseProxySetting(file.DefaultProxy); err != nil {
		return fmt.Errorf("%s: default proxy: %w", st.path, err)
	}
	st.defaultProxy = file.DefaultProxy
	return nil
}

func (st *settingsStore) save(defaultProxy string) error {
	if st.path == "" {
		return errors.New("no data directory")
	}
	b, err := json.MarshalIndent(settingsFile{Schema: SettingsSchema, DefaultProxy: defaultProxy}, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(st.path, ".settings-*.json", append(b, '\n'))
}

// writeFileAtomic writes a file with mode 0600 through a temporary file in
// the same directory and a rename.
func writeFileAtomic(path, pattern string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), pattern)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// parseProxySetting parses a proxy setting: nil for "" and "direct".
func parseProxySetting(v string) (*proxy.Config, error) {
	v = strings.TrimSpace(v)
	if v == "" || strings.EqualFold(v, model.ProxyDirect) {
		return nil, nil
	}
	if len(v) > model.MaxProxyLen {
		return nil, fmt.Errorf("a proxy URL has at most %d characters", model.MaxProxyLen)
	}
	c, err := proxy.Parse(v)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// normalProxySetting is the canonical form of a valid setting: "",
// "direct" or the URL as given (trimmed).
func normalProxySetting(v string) string {
	v = strings.TrimSpace(v)
	if strings.EqualFold(v, model.ProxyDirect) {
		return model.ProxyDirect
	}
	return v
}

// redactProxySetting replaces the password of a valid setting.
func redactProxySetting(v string) string {
	c, err := parseProxySetting(v)
	if err != nil || c == nil {
		if err != nil {
			return ""
		}
		return normalProxySetting(v)
	}
	return c.Redacted()
}

// Settings implements service.Service.
func (e *Engine) Settings(context.Context) (model.Settings, error) {
	if e.opts.DefaultProxy != "" {
		return model.Settings{DefaultProxy: redactProxySetting(e.opts.DefaultProxy), DefaultProxyLocked: true}, nil
	}
	e.settings.mu.Lock()
	defer e.settings.mu.Unlock()
	return model.Settings{DefaultProxy: e.settings.defaultProxy}, nil
}

// SaveSettings implements service.Service.
func (e *Engine) SaveSettings(ctx context.Context, s model.Settings) (model.Settings, error) {
	if e.opts.DefaultProxy != "" {
		return model.Settings{}, service.Errorf(service.CodeNotAllowed, "the default proxy is set by the daemon's operator (--proxy)")
	}
	if e.settings.path == "" {
		return model.Settings{}, service.Errorf(service.CodeNotAllowed, "this engine keeps no settings (no data directory)")
	}
	if _, err := parseProxySetting(s.DefaultProxy); err != nil {
		return model.Settings{}, service.Errorf(service.CodeInvalidArgument, "default proxy: %v", err)
	}
	v := normalProxySetting(s.DefaultProxy)
	e.settings.mu.Lock()
	defer e.settings.mu.Unlock()
	if err := e.settings.save(v); err != nil {
		return model.Settings{}, service.Errorf(service.CodeInternal, "cannot save the settings: %v", err)
	}
	e.settings.defaultProxy = v
	e.log.Info("default proxy changed", "proxy", redactProxySetting(v))
	return model.Settings{DefaultProxy: v}, nil
}

// proxyChoice is how a session reaches its IED.
type proxyChoice struct {
	// cfg is nil for a direct connection.
	cfg *proxy.Config
	// trusted is set for the operator's default proxy, which the target
	// allow-list does not restrict.
	trusted bool
}

// chooseProxy resolves a target's proxy setting against the defaults.
func (e *Engine) chooseProxy(setting string) (proxyChoice, error) {
	if setting != "" {
		c, err := parseProxySetting(setting)
		if err != nil {
			return proxyChoice{}, service.Errorf(service.CodeInvalidArgument, "proxy: %v", err)
		}
		return proxyChoice{cfg: c}, nil
	}
	if e.opts.DefaultProxy != "" {
		c, err := parseProxySetting(e.opts.DefaultProxy)
		if err != nil {
			return proxyChoice{}, service.Errorf(service.CodeInternal, "the operator's default proxy: %v", err)
		}
		return proxyChoice{cfg: c, trusted: true}, nil
	}
	e.settings.mu.Lock()
	v := e.settings.defaultProxy
	e.settings.mu.Unlock()
	c, err := parseProxySetting(v)
	if err != nil {
		return proxyChoice{}, service.Errorf(service.CodeInternal, "the default proxy: %v", err)
	}
	return proxyChoice{cfg: c}, nil
}
