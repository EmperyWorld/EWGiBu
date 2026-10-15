// Package journal implements an append-only durable action journal (stdlib JSONL at journal.db).
package journal

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// ActionState is a journal action lifecycle state (ADR-008).
type ActionState string

const (
	StatePlanned   ActionState = "planned"
	StateStarted   ActionState = "started"
	StateSucceeded ActionState = "succeeded"
	StateFailed    ActionState = "failed"
	StateUnknown   ActionState = "unknown"
)

// Record is one journal transition.
type Record struct {
	Seq        int64             `json:"seq"`
	RunID      string            `json:"runId"`
	ActionID   string            `json:"actionId"`
	State      ActionState       `json:"state"`
	Reversible bool              `json:"reversible"`
	Owned      []string          `json:"owned,omitempty"`
	Error      string            `json:"error,omitempty"` // redacted
	At         string            `json:"at"`              // RFC3339 UTC; recorded for operators, not used in digests
	Meta       map[string]string `json:"meta,omitempty"`
}

// Store is an append-only journal file.
type Store struct {
	path string
	mu   sync.Mutex
	seq  int64
	f    *os.File
}

// Open creates or opens a journal at path (mode 0600).
func Open(path string) (*Store, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	st := &Store{path: path, f: f}
	// recover seq
	raw, _ := os.ReadFile(path)
	for _, line := range splitLines(raw) {
		var r Record
		if json.Unmarshal(line, &r) == nil && r.Seq > st.seq {
			st.seq = r.Seq
		}
	}
	return st, nil
}

// Close closes the underlying file.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.f == nil {
		return nil
	}
	err := s.f.Close()
	s.f = nil
	return err
}

// Append writes one transition and fsyncs.
func (s *Store) Append(r Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	r.Seq = s.seq
	if r.At == "" {
		r.At = time.Now().UTC().Format(time.RFC3339Nano)
	}
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if _, err := s.f.Write(b); err != nil {
		return err
	}
	return s.f.Sync()
}

// Transition records planned→started→terminal for an action.
func (s *Store) Transition(runID, actionID string, reversible bool, state ActionState, owned []string, errMsg string) error {
	return s.Append(Record{
		RunID: runID, ActionID: actionID, State: state, Reversible: reversible, Owned: owned, Error: errMsg,
	})
}

// LastByAction returns the latest record per action ID for a run.
func (s *Store) LastByAction(runID string) (map[string]Record, error) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		return nil, err
	}
	out := map[string]Record{}
	for _, line := range splitLines(raw) {
		var r Record
		if err := json.Unmarshal(line, &r); err != nil {
			continue
		}
		if runID != "" && r.RunID != runID {
			continue
		}
		out[r.ActionID] = r
	}
	return out, nil
}

// DetectUnknown reports actions left in started without terminal state (crash).
func DetectUnknown(last map[string]Record) []string {
	var out []string
	for id, r := range last {
		if r.State == StateStarted {
			out = append(out, id)
		}
	}
	return out
}

func splitLines(b []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, c := range b {
		if c == '\n' {
			if i > start {
				lines = append(lines, b[start:i])
			}
			start = i + 1
		}
	}
	if start < len(b) {
		lines = append(lines, b[start:])
	}
	return lines
}

// NewRunID returns a deterministic-enough run id from counter+time for operators.
func NewRunID(n int64) string {
	return fmt.Sprintf("run-%d-%d", n, time.Now().UTC().UnixNano())
}
