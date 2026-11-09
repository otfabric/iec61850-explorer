// SPDX-License-Identifier: MIT

package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/otfabric/go-iec61850-model/catalog"
	gomodel "github.com/otfabric/go-iec61850-model/model"
	"github.com/otfabric/go-iec61850-model/nsd"
	"github.com/otfabric/go-iec61850-model/official"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
)

// nsRegistry holds what models are checked against: the releases embedded
// in go-iec61850-model and the packages a user loaded (official releases
// not embedded, vendor extension namespaces), kept as files in
// <data-dir>/namespaces so that they survive a restart. The catalog of
// both and the profiles chosen per declaration are built on first use and
// rebuilt when a package is added or removed.
type nsRegistry struct {
	dir string // "" keeps loaded packages in memory only

	mu       sync.Mutex
	loaded   []loadedPackage
	cat      *catalog.Catalog
	profiles map[string]resolvedProfile
}

type loadedPackage struct {
	info       model.NamespacePackage
	namespaces []*gomodel.Namespace
}

// Namespaces implements catalog.Source.
func (lp loadedPackage) Namespaces() []*gomodel.Namespace { return lp.namespaces }

// resolvedProfile is the profile for one declaration and what it says about
// the declaration.
type resolvedProfile struct {
	profile *catalog.Profile
	decl    model.DeclaredNamespace
}

// The embedded catalog and default profile, loaded once (about 65 ms).
// Tests replace loadEmbedded.
var (
	embeddedOnce sync.Once
	embeddedCat  *catalog.Catalog
	embeddedErr  error
	loadEmbedded = official.Catalog
)

func embedded() (*catalog.Catalog, error) {
	embeddedOnce.Do(func() { embeddedCat, embeddedErr = loadEmbedded() })
	if embeddedErr != nil {
		return nil, service.Errorf(service.CodeInternal, "the embedded IEC 61850 namespaces: %v", embeddedErr)
	}
	return embeddedCat, nil
}

func newNSRegistry(dataDir string, logf func(string, ...any)) *nsRegistry {
	r := &nsRegistry{}
	if dataDir == "" {
		return r
	}
	r.dir = filepath.Join(dataDir, "namespaces")
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			logf("namespaces: %v", err)
		}
		return r
	}
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		path := filepath.Join(r.dir, e.Name())
		data, err := readBounded(path, model.MaxNamespaceBytes)
		if err != nil {
			logf("namespaces: %s: %v", e.Name(), err)
			continue
		}
		lp, err := parsePackage(fileNameOf(e.Name()), data)
		if err != nil {
			logf("namespaces: %s: %v", e.Name(), err)
			continue
		}
		if info, err := e.Info(); err == nil {
			lp.info.AddedAt = info.ModTime().UTC()
		}
		r.loaded = append(r.loaded, lp)
	}
	return r
}

// A stored package is named "<sha256>--<original name>".
func fileNameOf(stored string) string {
	if _, name, ok := strings.Cut(stored, "--"); ok {
		return name
	}
	return stored
}

func readBounded(path string, limit int64) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > limit {
		return nil, fmt.Errorf("larger than %d MiB", limit>>20)
	}
	return os.ReadFile(filepath.Clean(path)) // a file of the engine's own data directory
}

// parsePackage reads a ZIP archive (namespaces with their documentation
// texts) or one NSD document, with go-iec61850-model's bounded importer.
func parsePackage(name string, data []byte) (loadedPackage, error) {
	var namespaces []*gomodel.Namespace
	docs := false
	if bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		pkg, err := nsd.ReadZIP(bytes.NewReader(data))
		if err != nil {
			return loadedPackage{}, err
		}
		namespaces, docs = pkg.Namespaces(), len(pkg.Docs()) > 0
	} else {
		doc, err := nsd.Parse(bytes.NewReader(data))
		if err != nil {
			return loadedPackage{}, err
		}
		if doc.Namespace != nil {
			namespaces = []*gomodel.Namespace{doc.Namespace}
		}
	}
	if len(namespaces) == 0 {
		return loadedPackage{}, errors.New("it defines no namespace (documentation or a service namespace alone adds nothing to check against)")
	}
	sum := sha256.Sum256(data)
	info := model.NamespacePackage{ID: hex.EncodeToString(sum[:6]), File: name, SHA256: hex.EncodeToString(sum[:]),
		AddedAt: time.Now().UTC(), Docs: docs}
	for _, ns := range namespaces {
		info.Namespaces = append(info.Namespaces, ns.ID.String())
	}
	return loadedPackage{info: info, namespaces: namespaces}, nil
}

// catalogLocked builds the catalog of the embedded and loaded namespaces.
func (r *nsRegistry) catalogLocked() (*catalog.Catalog, error) {
	if r.cat != nil {
		return r.cat, nil
	}
	base, err := embedded()
	if err != nil {
		return nil, err
	}
	sources := []catalog.Source{base}
	for _, lp := range r.loaded {
		sources = append(sources, lp)
	}
	cat, err := catalog.New(sources...)
	if err != nil {
		return nil, service.Errorf(service.CodeInternal, "the namespaces: %v", err)
	}
	r.cat, r.profiles = cat, map[string]resolvedProfile{}
	return cat, nil
}

// profileFor gives the profile to check a declaration against. A loaded
// package's namespace wins; then the embedded profile go-iec61850-model
// chooses for it (official.ForNamespace). IEC publishes no namespace
// definitions for Edition 1 of IEC 61850-7-4 (2003): such a device is
// checked against the default profile and marked. An empty declaration
// uses the default profile; one that is neither loaded nor embedded too,
// with a note.
func (r *nsRegistry) profileFor(decl string) (*catalog.Profile, model.DeclaredNamespace, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cat, err := r.catalogLocked()
	if err != nil {
		return nil, model.DeclaredNamespace{}, err
	}
	decl = strings.TrimSpace(decl)
	if rp, ok := r.profiles[decl]; ok {
		return rp.profile, rp.decl, nil
	}
	out := model.DeclaredNamespace{Declared: decl}
	ids, note := r.selectLocked(decl)
	switch note {
	case noteEdition1:
		out.Edition1 = true
		out.Note = "IEC 61850-7-4 Edition 1 (2003): IEC publishes no namespace definitions for it; checked against Edition 2, and what Edition 2 requires is reported as a warning"
	case noteUnknown:
		out.Unknown = true
		out.Note = fmt.Sprintf("%s is neither embedded nor loaded: checked against the default profile; load its definitions to check against them", decl)
	}
	p, err := cat.Profile(ids...)
	if err != nil {
		// A loaded release whose dependencies are not there. The default
		// profile is embedded and complete.
		out.Note = fmt.Sprintf("%s: %v; checked against the default profile", decl, err)
		p, _ = cat.Profile(defaultIDs()...)
	}
	for _, id := range p.IDs() {
		out.CheckedAgainst = append(out.CheckedAgainst, id.String())
	}
	r.profiles[decl] = resolvedProfile{profile: p, decl: out}
	return p, out, nil
}

const (
	noteNone = iota
	noteEdition1
	noteUnknown
)

func defaultIDs() []gomodel.NamespaceID {
	p, _ := official.Lookup("default")
	return p.Namespaces
}

// selectLocked picks the releases for a declaration.
func (r *nsRegistry) selectLocked(decl string) ([]gomodel.NamespaceID, int) {
	if decl == "" {
		return defaultIDs(), noteNone
	}
	want, err := gomodel.ParseNamespaceID(decl)
	if err != nil {
		return defaultIDs(), noteUnknown
	}
	var best *gomodel.NamespaceID
	for _, lp := range r.loaded {
		for _, ns := range lp.namespaces {
			if declares(want, ns.ID) && (best == nil || ns.ID.Release > best.Release) {
				id := ns.ID
				best = &id
			}
		}
	}
	if best != nil {
		return []gomodel.NamespaceID{*best}, noteNone
	}
	if p, err := official.ForNamespace(decl); err == nil {
		return p.Namespaces, noteNone
	}
	if normStandard(want.Standard) == normStandard("IEC 61850-7-4") && want.Version == "2003" {
		return defaultIDs(), noteEdition1
	}
	return defaultIDs(), noteUnknown
}

// declares reports whether a declaration names a release: the same
// standard, the declared version and revision when given, at least the
// declared release.
func declares(want, have gomodel.NamespaceID) bool {
	return normStandard(want.Standard) == normStandard(have.Standard) &&
		(want.Version == "" || want.Version == have.Version) &&
		(want.Revision == "" || want.Revision == have.Revision) &&
		have.Release >= want.Release
}

// normStandard compares standards without spaces, case and technical-report
// markers: "IEC TR 61850-90-9" is "(Tr)IEC 61850-90-9".
func normStandard(s string) string {
	s = strings.NewReplacer(" ", "", "(tr)", "").Replace(strings.ToLower(s))
	return strings.Replace(s, "iectr", "iec", 1)
}

// Namespaces implements service.Service.
func (e *Engine) Namespaces(context.Context) (model.Namespaces, error) {
	out := model.Namespaces{Embedded: []string{}, Profiles: []model.NamespaceProfile{}, Loaded: []model.NamespacePackage{}}
	for _, id := range official.Namespaces() {
		out.Embedded = append(out.Embedded, id.String())
	}
	for _, p := range official.Profiles() {
		np := model.NamespaceProfile{Name: p.Name, Description: p.Description}
		for _, id := range p.Namespaces {
			np.Namespaces = append(np.Namespaces, id.String())
		}
		out.Profiles = append(out.Profiles, np)
	}
	e.ns.mu.Lock()
	defer e.ns.mu.Unlock()
	for _, lp := range e.ns.loaded {
		out.Loaded = append(out.Loaded, lp.info)
	}
	return out, nil
}

// AddNamespaces implements service.Service: it loads a package of namespace
// definitions (a ZIP archive or one NSD document) and keeps it.
func (e *Engine) AddNamespaces(_ context.Context, f model.File) (model.NamespacePackage, error) {
	if len(f.Data) == 0 {
		return model.NamespacePackage{}, service.Errorf(service.CodeInvalidArgument, "the namespace package is empty")
	}
	if len(f.Data) > model.MaxNamespaceBytes {
		return model.NamespacePackage{}, service.Errorf(service.CodeLimitExceeded, "a namespace package is at most %d MiB", model.MaxNamespaceBytes>>20)
	}
	name := displayName(f.Name)
	lp, err := parsePackage(name, f.Data)
	if err != nil {
		return model.NamespacePackage{}, service.Errorf(service.CodeInvalidArgument, "%s: %v", name, err)
	}
	r := e.ns
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.loaded) >= model.MaxNamespacePackages {
		return model.NamespacePackage{}, service.Errorf(service.CodeLimitExceeded, "at most %d namespace packages", model.MaxNamespacePackages)
	}
	base, err := embedded()
	if err != nil {
		return model.NamespacePackage{}, err
	}
	sources := []catalog.Source{base, lp}
	for _, other := range r.loaded {
		sources = append(sources, other)
	}
	if _, err := catalog.New(sources...); err != nil {
		return model.NamespacePackage{}, service.Errorf(service.CodeInvalidArgument, "%s: %v (a release that is embedded or loaded already)", name, err)
	}
	if r.dir != "" {
		if err := os.MkdirAll(r.dir, 0o700); err != nil {
			return model.NamespacePackage{}, service.Errorf(service.CodeInternal, "namespaces: %v", err)
		}
		stored := filepath.Join(r.dir, lp.info.SHA256+"--"+unsafeName.ReplaceAllString(name, "_"))
		if err := os.WriteFile(stored, f.Data, 0o600); err != nil {
			return model.NamespacePackage{}, service.Errorf(service.CodeInternal, "namespaces: %v", err)
		}
	}
	r.loaded = append(r.loaded, lp)
	r.cat, r.profiles = nil, nil
	return lp.info, nil
}

// RemoveNamespaces implements service.Service.
func (e *Engine) RemoveNamespaces(_ context.Context, id string) error {
	r := e.ns
	r.mu.Lock()
	defer r.mu.Unlock()
	i := slices.IndexFunc(r.loaded, func(lp loadedPackage) bool { return lp.info.ID == id })
	if i < 0 {
		return service.Errorf(service.CodeNotFound, "no loaded namespace package %q", id)
	}
	if r.dir != "" {
		matches, _ := filepath.Glob(filepath.Join(r.dir, r.loaded[i].info.SHA256+"--*"))
		for _, m := range matches {
			if err := os.Remove(m); err != nil {
				return service.Errorf(service.CodeInternal, "namespaces: %v", err)
			}
		}
	}
	r.loaded = slices.Delete(r.loaded, i, i+1)
	r.cat, r.profiles = nil, nil
	return nil
}
