// SPDX-License-Identifier: MIT

package engine

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/otfabric/go-mms"

	"github.com/otfabric/iec61850-explorer/internal/model"
	"github.com/otfabric/iec61850-explorer/internal/service"
)

// ProfilesSchema identifies the profiles file.
const ProfilesSchema = "otfabric.iec61850-explorer.profiles/1"

// maxProfilesFile bounds the profiles file the engine reads.
const maxProfilesFile = 4 << 20

type profilesFile struct {
	Schema   string          `json:"schema"`
	Profiles []model.Profile `json:"profiles"`
}

// profileStore keeps connection profiles in one JSON file, written whole and
// atomically (temporary file, then rename), mode 0600.
type profileStore struct {
	path string
	mu   sync.Mutex
}

func (p *profileStore) load() ([]model.Profile, error) {
	f, err := os.Open(p.path)
	if errors.Is(err, fs.ErrNotExist) {
		return []model.Profile{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > maxProfilesFile {
		return nil, fmt.Errorf("%s is larger than %d bytes", p.path, maxProfilesFile)
	}
	var file profilesFile
	if err := json.NewDecoder(f).Decode(&file); err != nil {
		return nil, fmt.Errorf("%s: %w", p.path, err)
	}
	if file.Schema != ProfilesSchema {
		return nil, fmt.Errorf("%s: schema %q, want %q", p.path, file.Schema, ProfilesSchema)
	}
	if file.Profiles == nil {
		file.Profiles = []model.Profile{}
	}
	return file.Profiles, nil
}

func (p *profileStore) save(list []model.Profile) error {
	b, err := json.MarshalIndent(profilesFile{Schema: ProfilesSchema, Profiles: list}, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(p.path, ".profiles-*.json", append(b, '\n'))
}

func (e *Engine) profiles() (*profileStore, error) {
	if e.store == nil {
		return nil, service.Errorf(service.CodeNotAllowed, "this engine keeps no profiles (no data directory)")
	}
	return e.store, nil
}

// Profiles implements service.Service.
func (e *Engine) Profiles(context.Context) ([]model.Profile, error) {
	store, err := e.profiles()
	if err != nil {
		return nil, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	list, err := store.load()
	if err != nil {
		return nil, service.Errorf(service.CodeInternal, "cannot read the profiles: %v", err)
	}
	slices.SortFunc(list, func(a, b model.Profile) int {
		return cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return list, nil
}

// SaveProfile implements service.Service.
func (e *Engine) SaveProfile(_ context.Context, p model.Profile) (model.Profile, error) {
	store, err := e.profiles()
	if err != nil {
		return model.Profile{}, err
	}
	p.Name = strings.TrimSpace(p.Name)
	switch {
	case p.Name == "" || len(p.Name) > model.MaxProfileNameLen:
		return model.Profile{}, service.Errorf(service.CodeInvalidArgument, "a profile name has 1 to %d characters", model.MaxProfileNameLen)
	case strings.IndexFunc(p.Name, unicode.IsControl) >= 0 || strings.IndexFunc(p.Notes, isBadNoteRune) >= 0:
		return model.Profile{}, service.Errorf(service.CodeInvalidArgument, "the profile contains control characters")
	case len(p.Notes) > model.MaxProfileNotesLen:
		return model.Profile{}, service.Errorf(service.CodeInvalidArgument, "notes have at most %d characters", model.MaxProfileNotesLen)
	}
	cfg, _, err := validateTarget(p.Target)
	if err != nil {
		return model.Profile{}, err
	}
	if err := applyAddressing(cfg.Addressing, &mms.DialOptions{}); err != nil {
		return model.Profile{}, err
	}
	p.Target = cfg
	p.UpdatedAt = time.Now().UTC()

	store.mu.Lock()
	defer store.mu.Unlock()
	list, err := store.load()
	if err != nil {
		return model.Profile{}, service.Errorf(service.CodeInternal, "cannot read the profiles: %v", err)
	}
	if p.ID == "" {
		if len(list) >= model.MaxProfiles {
			return model.Profile{}, service.Errorf(service.CodeLimitExceeded, "at most %d profiles", model.MaxProfiles)
		}
		p.ID = newID()
		list = append(list, p)
	} else {
		i := slices.IndexFunc(list, func(q model.Profile) bool { return q.ID == p.ID })
		if i < 0 {
			return model.Profile{}, service.Errorf(service.CodeNotFound, "no profile %q", p.ID)
		}
		list[i] = p
	}
	if err := store.save(list); err != nil {
		return model.Profile{}, service.Errorf(service.CodeInternal, "cannot save the profiles: %v", err)
	}
	return p, nil
}

// DeleteProfile implements service.Service.
func (e *Engine) DeleteProfile(_ context.Context, id string) error {
	store, err := e.profiles()
	if err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	list, err := store.load()
	if err != nil {
		return service.Errorf(service.CodeInternal, "cannot read the profiles: %v", err)
	}
	i := slices.IndexFunc(list, func(q model.Profile) bool { return q.ID == id })
	if i < 0 {
		return service.Errorf(service.CodeNotFound, "no profile %q", id)
	}
	if err := store.save(slices.Delete(list, i, i+1)); err != nil {
		return service.Errorf(service.CodeInternal, "cannot save the profiles: %v", err)
	}
	return nil
}

// isBadNoteRune allows line breaks and tabs in notes, no other control
// characters.
func isBadNoteRune(r rune) bool {
	return unicode.IsControl(r) && r != '\n' && r != '\t'
}
