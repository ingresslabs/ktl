package mcpserver

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type sessionState string

const (
	sessionQueued    sessionState = "queued"
	sessionRunning   sessionState = "running"
	sessionSucceeded sessionState = "succeeded"
	sessionFailed    sessionState = "failed"
	sessionCancelled sessionState = "cancelled"
)

type sessionRecord struct {
	SessionID    string         `json:"sessionId"`
	Kind         string         `json:"kind"`
	State        sessionState   `json:"state"`
	CreatedAt    time.Time      `json:"createdAt"`
	UpdatedAt    time.Time      `json:"updatedAt"`
	Requester    string         `json:"requester,omitempty"`
	Remote       bool           `json:"remote,omitempty"`
	Error        string         `json:"error,omitempty"`
	Summary      map[string]any `json:"summary,omitempty"`
	LastSequence uint64         `json:"lastSequence,omitempty"`
}

type sessionEvent struct {
	Sequence uint64         `json:"sequence"`
	Time     time.Time      `json:"time"`
	Kind     string         `json:"kind"`
	Message  string         `json:"message"`
	Fields   map[string]any `json:"fields,omitempty"`
}

type sessionStore struct {
	mu       sync.Mutex
	next     uint64
	sessions map[string]*sessionRecord
	events   map[string][]sessionEvent
}

func newSessionStore() *sessionStore {
	return &sessionStore{
		sessions: map[string]*sessionRecord{},
		events:   map[string][]sessionEvent{},
	}
}

func (s *sessionStore) create(kind, requester string, remote bool) *sessionRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	now := time.Now().UTC()
	id := fmt.Sprintf("mcp_%s_%06d", sanitizeID(kind), s.next)
	rec := &sessionRecord{
		SessionID: id,
		Kind:      strings.TrimSpace(kind),
		State:     sessionQueued,
		CreatedAt: now,
		UpdatedAt: now,
		Requester: strings.TrimSpace(requester),
		Remote:    remote,
	}
	s.sessions[id] = rec
	return cloneSession(rec)
}

func (s *sessionStore) upsert(rec *sessionRecord) {
	if rec == nil || strings.TrimSpace(rec.SessionID) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := cloneSession(rec)
	if cp.UpdatedAt.IsZero() {
		cp.UpdatedAt = time.Now().UTC()
	}
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = cp.UpdatedAt
	}
	s.sessions[cp.SessionID] = cp
}

func (s *sessionStore) update(id string, state sessionState, errText string, summary map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec := s.sessions[id]
	if rec == nil {
		return
	}
	rec.State = state
	rec.Error = strings.TrimSpace(errText)
	rec.UpdatedAt = time.Now().UTC()
	if summary != nil {
		rec.Summary = summary
	}
}

func (s *sessionStore) appendEvent(id, kind, message string, fields map[string]any) sessionEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec := s.sessions[id]
	if rec == nil {
		rec = &sessionRecord{SessionID: id, Kind: "remote", State: sessionRunning, CreatedAt: time.Now().UTC()}
		s.sessions[id] = rec
	}
	seq := rec.LastSequence + 1
	ev := sessionEvent{
		Sequence: seq,
		Time:     time.Now().UTC(),
		Kind:     strings.TrimSpace(kind),
		Message:  strings.TrimSpace(message),
		Fields:   fields,
	}
	s.events[id] = append(s.events[id], ev)
	rec.LastSequence = seq
	rec.UpdatedAt = ev.Time
	return ev
}

func (s *sessionStore) list() []sessionRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]sessionRecord, 0, len(s.sessions))
	for _, rec := range s.sessions {
		out = append(out, *cloneSession(rec))
	}
	return out
}

func (s *sessionStore) get(id string) (*sessionRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec := s.sessions[strings.TrimSpace(id)]
	if rec == nil {
		return nil, false
	}
	return cloneSession(rec), true
}

func (s *sessionStore) tail(id string, from uint64, limit int) []sessionEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 {
		limit = 100
	}
	events := s.events[strings.TrimSpace(id)]
	out := make([]sessionEvent, 0, minInt(limit, len(events)))
	for _, ev := range events {
		if ev.Sequence <= from {
			continue
		}
		out = append(out, ev)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func cloneSession(rec *sessionRecord) *sessionRecord {
	if rec == nil {
		return nil
	}
	cp := *rec
	if rec.Summary != nil {
		cp.Summary = map[string]any{}
		for k, v := range rec.Summary {
			cp.Summary[k] = v
		}
	}
	return &cp
}

func sanitizeID(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			if b.Len() > 0 {
				b.WriteByte('_')
			}
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "session"
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
