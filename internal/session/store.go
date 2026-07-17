package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Store persists sessions under a root directory (one JSON file per session).
type Store struct {
	Root string
}

// NewStore creates a store rooted at dir.
func NewStore(root string) *Store {
	return &Store{Root: root}
}

func (st *Store) pathFor(id string) string {
	// Sanitize id to a single path segment.
	safe := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return -1
	}, id)
	if safe == "" {
		safe = "session"
	}
	return filepath.Join(st.Root, safe+".json")
}

// Save writes the session atomically.
func (st *Store) Save(s *Session) error {
	if s == nil {
		return fmt.Errorf("nil session")
	}
	if err := os.MkdirAll(st.Root, 0o700); err != nil {
		return fmt.Errorf("mkdir sessions: %w", err)
	}
	s.UpdatedAt = time.Now().UTC()
	if s.SchemaVer == 0 {
		s.SchemaVer = schemaVersion
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encode session: %w", err)
	}
	data = append(data, '\n')
	path := st.pathFor(s.ID)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write session tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename session: %w", err)
	}
	return nil
}

// Load reads a session by id and applies most-recently-changed tip selection.
func (st *Store) Load(id string) (*Session, error) {
	path := st.pathFor(id)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read session: %w", err)
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse session: %w", err)
	}
	if s.Turns == nil {
		s.Turns = make(map[string]Turn)
	}
	s.ApplyLoadDefaults()
	return &s, nil
}

// List returns summaries of all sessions, newest first.
func (st *Store) List() ([]Summary, error) {
	entries, err := os.ReadDir(st.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	var out []Summary
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(st.Root, e.Name()))
		if err != nil {
			continue
		}
		var s Session
		if err := json.Unmarshal(data, &s); err != nil {
			continue
		}
		out = append(out, s.ToSummary())
	}
	slices.SortFunc(out, func(a, b Summary) int {
		if a.UpdatedAt.After(b.UpdatedAt) {
			return -1
		}
		if b.UpdatedAt.After(a.UpdatedAt) {
			return 1
		}
		return strings.Compare(a.Title, b.Title)
	})
	return out, nil
}

// Search loads sessions and returns those matching query (title/content).
func (st *Store) Search(query string) ([]Summary, error) {
	entries, err := os.ReadDir(st.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	var out []Summary
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(st.Root, e.Name()))
		if err != nil {
			continue
		}
		var s Session
		if err := json.Unmarshal(data, &s); err != nil {
			continue
		}
		if s.MatchesQuery(query) {
			out = append(out, s.ToSummary())
		}
	}
	slices.SortFunc(out, func(a, b Summary) int {
		if a.UpdatedAt.After(b.UpdatedAt) {
			return -1
		}
		if b.UpdatedAt.After(a.UpdatedAt) {
			return 1
		}
		return strings.Compare(a.Title, b.Title)
	})
	return out, nil
}

// Delete removes a session file.
func (st *Store) Delete(id string) error {
	path := st.pathFor(id)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}
