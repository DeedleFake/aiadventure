// Package session implements adventure session trees, branching, and phases.
package session

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Phase is the current stage of an adventure session.
type Phase string

const (
	PhaseBrainstorm Phase = "brainstorm"
	PhaseAdventure  Phase = "adventure"
)

// Role identifies who produced a turn.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Turn is one message in the session tree.
type Turn struct {
	ID        string    `json:"id"`
	ParentID  string    `json:"parent_id,omitempty"`
	Role      Role      `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// Feedback is out-of-band guidance for future AI responses (not story turns).
type Feedback struct {
	ID        string    `json:"id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// Session is a branched adventure conversation.
type Session struct {
	ID          string          `json:"id"`
	Title       string          `json:"title"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	Phase       Phase           `json:"phase"`
	Model       string          `json:"model,omitempty"`
	Effort      string          `json:"effort,omitempty"`
	Turns       map[string]Turn `json:"turns"`
	ActiveTipID string          `json:"active_tip_id,omitempty"`
	Feedback    []Feedback      `json:"feedback,omitempty"`
	SchemaVer   int             `json:"schema_version"`
}

const schemaVersion = 1

// DefaultTitle is used when no title has been derived yet.
const DefaultTitle = "Untitled adventure"

// New creates a session in the brainstorm phase.
func New(title, model, effort string) *Session {
	now := time.Now().UTC()
	if title == "" {
		title = DefaultTitle
	}
	return &Session{
		ID:        newID(),
		Title:     title,
		CreatedAt: now,
		UpdatedAt: now,
		Phase:     PhaseBrainstorm,
		Model:     model,
		Effort:    effort,
		Turns:     make(map[string]Turn),
		Feedback:  nil,
		SchemaVer: schemaVersion,
	}
}

// AutoTitleFromText derives a short session title from the first user message.
// Uses the first line, collapses whitespace, and truncates to a display-friendly length.
func AutoTitleFromText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return DefaultTitle
	}
	if i := strings.IndexAny(text, "\n\r"); i >= 0 {
		text = strings.TrimSpace(text[:i])
	}
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return DefaultTitle
	}
	text = strings.Join(fields, " ")
	const maxRunes = 60
	r := []rune(text)
	if len(r) > maxRunes {
		return string(r[:maxRunes-1]) + "…"
	}
	return text
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Extremely unlikely; fall back to timestamp-ish uniqueness.
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// Append adds a turn on the active branch tip and returns its id.
func (s *Session) Append(role Role, content string) (string, error) {
	if s.Turns == nil {
		s.Turns = make(map[string]Turn)
	}
	now := time.Now().UTC()
	t := Turn{
		ID:        newID(),
		ParentID:  s.ActiveTipID,
		Role:      role,
		Content:   content,
		CreatedAt: now,
	}
	s.Turns[t.ID] = t
	s.ActiveTipID = t.ID
	s.UpdatedAt = now
	return t.ID, nil
}

// Path returns the root-to-tip chain for the given tip id (or active tip).
func (s *Session) Path(tipID string) []Turn {
	if tipID == "" {
		tipID = s.ActiveTipID
	}
	if tipID == "" {
		return nil
	}
	var rev []Turn
	seen := make(map[string]bool)
	for id := tipID; id != ""; {
		if seen[id] {
			break // cycle guard
		}
		seen[id] = true
		t, ok := s.Turns[id]
		if !ok {
			break
		}
		rev = append(rev, t)
		id = t.ParentID
	}
	slices.Reverse(rev)
	return rev
}

// ActivePath is Path(ActiveTipID).
func (s *Session) ActivePath() []Turn {
	return s.Path(s.ActiveTipID)
}

// LeafTips returns ids of all turns that have no children.
func (s *Session) LeafTips() []string {
	hasChild := make(map[string]bool)
	for _, t := range s.Turns {
		if t.ParentID != "" {
			hasChild[t.ParentID] = true
		}
	}
	var tips []string
	for id := range s.Turns {
		if !hasChild[id] {
			tips = append(tips, id)
		}
	}
	slices.Sort(tips)
	return tips
}

// BranchTipActivity returns the latest CreatedAt along the path to tipID.
func (s *Session) BranchTipActivity(tipID string) time.Time {
	var latest time.Time
	for _, t := range s.Path(tipID) {
		if t.CreatedAt.After(latest) {
			latest = t.CreatedAt
		}
	}
	return latest
}

// MostRecentlyChangedTip picks the leaf whose branch has the latest activity.
// Empty string if there are no turns.
func (s *Session) MostRecentlyChangedTip() string {
	tips := s.LeafTips()
	if len(tips) == 0 {
		return ""
	}
	best := tips[0]
	bestTime := s.BranchTipActivity(best)
	for _, id := range tips[1:] {
		t := s.BranchTipActivity(id)
		if t.After(bestTime) {
			best = id
			bestTime = t
		}
	}
	return best
}

// SelectTip sets the active tip if it is a known turn id.
func (s *Session) SelectTip(tipID string) error {
	if _, ok := s.Turns[tipID]; !ok {
		return fmt.Errorf("unknown tip %q", tipID)
	}
	s.ActiveTipID = tipID
	s.UpdatedAt = time.Now().UTC()
	return nil
}

// ApplyLoadDefaults sets active tip to most recently changed branch when loading.
func (s *Session) ApplyLoadDefaults() {
	if tip := s.MostRecentlyChangedTip(); tip != "" {
		s.ActiveTipID = tip
	}
	if s.Turns == nil {
		s.Turns = make(map[string]Turn)
	}
}

// EditTurn forks a new branch by replacing content of turnID (same parent).
// The new turn becomes the active tip (branch diverges from that point).
func (s *Session) EditTurn(turnID, newContent string) (string, error) {
	orig, ok := s.Turns[turnID]
	if !ok {
		return "", fmt.Errorf("unknown turn %q", turnID)
	}
	if s.Turns == nil {
		s.Turns = make(map[string]Turn)
	}
	now := time.Now().UTC()
	t := Turn{
		ID:        newID(),
		ParentID:  orig.ParentID,
		Role:      orig.Role,
		Content:   newContent,
		CreatedAt: now,
	}
	s.Turns[t.ID] = t
	s.ActiveTipID = t.ID
	s.UpdatedAt = now
	return t.ID, nil
}

// SetPhase transitions brainstorm ↔ adventure.
func (s *Session) SetPhase(p Phase) error {
	switch p {
	case PhaseBrainstorm, PhaseAdventure:
		s.Phase = p
		s.UpdatedAt = time.Now().UTC()
		return nil
	default:
		return fmt.Errorf("invalid phase %q", p)
	}
}

// AddFeedback appends out-of-band future-response guidance without mutating turns.
func (s *Session) AddFeedback(content string) Feedback {
	now := time.Now().UTC()
	f := Feedback{
		ID:        newID(),
		Content:   content,
		CreatedAt: now,
	}
	s.Feedback = append(s.Feedback, f)
	s.UpdatedAt = now
	return f
}

// TurnSnapshot returns a copy of the turns that existed before a mutation,
// for tests that verify OOB feedback does not alter story turns.
func (s *Session) TurnIDs() []string {
	ids := make([]string, 0, len(s.Turns))
	for id := range s.Turns {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// MatchesQuery reports whether title or any turn content matches q (case-insensitive substring).
func (s *Session) MatchesQuery(q string) bool {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return true
	}
	if strings.Contains(strings.ToLower(s.Title), q) {
		return true
	}
	if strings.Contains(strings.ToLower(string(s.Phase)), q) {
		return true
	}
	for _, t := range s.Turns {
		if strings.Contains(strings.ToLower(t.Content), q) {
			return true
		}
	}
	for _, f := range s.Feedback {
		if strings.Contains(strings.ToLower(f.Content), q) {
			return true
		}
	}
	return false
}

// Summary is a lightweight listing row.
type Summary struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Phase     Phase     `json:"phase"`
	UpdatedAt time.Time `json:"updated_at"`
	CreatedAt time.Time `json:"created_at"`
}

// ToSummary returns listing metadata.
func (s *Session) ToSummary() Summary {
	return Summary{
		ID:        s.ID,
		Title:     s.Title,
		Phase:     s.Phase,
		UpdatedAt: s.UpdatedAt,
		CreatedAt: s.CreatedAt,
	}
}
