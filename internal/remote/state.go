package remote

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const maxReceipts = 32

// Receipt lets a gateway retry the same inbound ID without advancing twice.
// It is not an outbound carrier delivery receipt or a transactional outbox.
type Receipt struct {
	ID       string `json:"id"`
	BodyHash string `json:"body_sha256"`
	Reply    string `json:"reply"`
}

type State struct {
	Current   string              `json:"current,omitempty"`
	Positions map[string]int      `json:"positions,omitempty"` // previous format, migrated when that book is opened
	Bookmarks map[string]bookmark `json:"bookmarks,omitempty"`
	Recent    []Receipt           `json:"recent,omitempty"`
}

// The range identifies the words the sender actually saw, including any
// change of reading length. Offsets are bytes in the normalized UTF-8 edition.
type span struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type bookmark struct {
	Source string `json:"source"`
	Page   span   `json:"page"`
	Trail  []span `json:"trail,omitempty"`
}

type Store struct {
	Dir string
}

func (s Store) path(sender string) string {
	h := sha256.Sum256([]byte(sender))
	return filepath.Join(s.Dir, "readers", hex.EncodeToString(h[:])+".json")
}

func (s Store) Load(sender string) (*State, error) {
	data, err := os.ReadFile(s.path(sender))
	if errors.Is(err, os.ErrNotExist) {
		return &State{Bookmarks: make(map[string]bookmark)}, nil
	}
	if err != nil {
		return nil, err
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("reading bookmark: %w", err)
	}
	if st.Bookmarks == nil {
		st.Bookmarks = make(map[string]bookmark)
	}
	return &st, nil
}

func (s Store) Save(sender string, st *State) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(s.path(sender), append(data, '\n'))
}

// atomicWrite uses fsync -> close -> rename in the target directory, then
// best-effort directory sync. File permissions are private even before rename.
func atomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".pending-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return err
	}
	if dir, err := os.Open(filepath.Dir(path)); err == nil {
		_ = dir.Sync() // Some filesystems do not support directory sync.
		_ = dir.Close()
	}
	return nil
}
