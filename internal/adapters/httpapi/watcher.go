package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ManifestEventKind matches OpenAPI ManifestEvent.kind.
type ManifestEventKind string

const (
	EventChanged          ManifestEventKind = "changed"
	EventInvalid          ManifestEventKind = "invalid"
	EventDeleted          ManifestEventKind = "deleted"
	EventPermissionDenied ManifestEventKind = "permissionDenied"
	EventResyncRequired   ManifestEventKind = "resyncRequired"
)

// ManifestEvent is the SSE payload (revision only; never file content).
type ManifestEvent struct {
	Kind     ManifestEventKind `json:"kind"`
	Revision string            `json:"revision"`
	ID       string            `json:"-"` // SSE id; not in JSON body
}

type sseClient struct {
	ch chan ManifestEvent
}

type bufferedEvent struct {
	id string
	ev ManifestEvent
}

// eventHub fans out manifest events to SSE subscribers with a bounded buffer.
type eventHub struct {
	mu      sync.Mutex
	clients map[*sseClient]struct{}
	buffer  []bufferedEvent
	maxBuf  int
	seq     uint64
}

func newEventHub(maxBuf int) *eventHub {
	if maxBuf <= 0 {
		maxBuf = 1000
	}
	return &eventHub{clients: map[*sseClient]struct{}{}, maxBuf: maxBuf}
}

func (h *eventHub) subscribe(lastEventID string) *sseClient {
	c := &sseClient{ch: make(chan ManifestEvent, 16)}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	backlog := h.replayLocked(lastEventID)
	h.mu.Unlock()
	go func() {
		for _, ev := range backlog {
			select {
			case c.ch <- ev:
			default:
				return
			}
		}
	}()
	return c
}

func (h *eventHub) replayLocked(lastEventID string) []ManifestEvent {
	if lastEventID == "" {
		out := make([]ManifestEvent, 0, len(h.buffer))
		for _, b := range h.buffer {
			out = append(out, b.ev)
		}
		return out
	}
	found := false
	out := []ManifestEvent{}
	for _, b := range h.buffer {
		if found {
			out = append(out, b.ev)
			continue
		}
		if b.id == lastEventID {
			found = true
		}
	}
	if !found {
		return []ManifestEvent{{Kind: EventResyncRequired, Revision: "missed", ID: "resync"}}
	}
	return out
}

func (h *eventHub) unsubscribe(c *sseClient) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
	close(c.ch)
}

func (h *eventHub) publish(ev ManifestEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.seq++
	id := formatEventID(h.seq)
	ev.ID = id
	h.buffer = append(h.buffer, bufferedEvent{id: id, ev: ev})
	var extra *ManifestEvent
	if len(h.buffer) > h.maxBuf {
		h.buffer = h.buffer[len(h.buffer)-h.maxBuf:]
		h.seq++
		resync := ManifestEvent{Kind: EventResyncRequired, Revision: "overflow", ID: formatEventID(h.seq)}
		h.buffer = append([]bufferedEvent{{id: resync.ID, ev: resync}}, h.buffer...)
		if len(h.buffer) > h.maxBuf {
			h.buffer = h.buffer[:h.maxBuf]
		}
		extra = &resync
	}
	deliver := []ManifestEvent{}
	if extra != nil {
		deliver = append(deliver, *extra)
	}
	deliver = append(deliver, ev)
	for c := range h.clients {
		for _, item := range deliver {
			select {
			case c.ch <- item:
			default:
			}
		}
	}
}

func formatEventID(n uint64) string {
	return strconv.FormatUint(n, 10)
}

func encodeManifestSSE(ev ManifestEvent) string {
	b, _ := json.Marshal(struct {
		Kind     ManifestEventKind `json:"kind"`
		Revision string            `json:"revision"`
	}{Kind: ev.Kind, Revision: ev.Revision})
	id := ev.ID
	if id == "" {
		id = ev.Revision
	}
	return "id: " + id + "\nevent: manifest\ndata: " + string(b) + "\n\n"
}

type fileSnapshot struct {
	exists bool
	permOK bool
	rev    string
	rawOK  bool
	valid  bool
}

type manifestWatcher struct {
	path     string
	hub      *eventHub
	interval time.Duration
	debounce time.Duration

	mu           sync.Mutex
	last         fileSnapshot
	selfRevs     map[string]time.Time
	pending      *fileSnapshot
	pendingSince time.Time
}

func newManifestWatcher(path string, hub *eventHub) *manifestWatcher {
	return &manifestWatcher{
		path:     path,
		hub:      hub,
		interval: 200 * time.Millisecond,
		debounce: 350 * time.Millisecond,
		selfRevs: map[string]time.Time{},
	}
}

func (w *manifestWatcher) noteSelfWrite(revision string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := time.Now()
	w.selfRevs[revision] = now.Add(3 * time.Second)
	for r, exp := range w.selfRevs {
		if now.After(exp) {
			delete(w.selfRevs, r)
		}
	}
}

func (w *manifestWatcher) isSelfLocked(revision string) bool {
	exp, ok := w.selfRevs[revision]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(w.selfRevs, revision)
		return false
	}
	delete(w.selfRevs, revision)
	return true
}

func (w *manifestWatcher) setPath(path string) {
	w.mu.Lock()
	w.path = path
	w.last = fileSnapshot{}
	w.pending = nil
	w.mu.Unlock()
}

func (w *manifestWatcher) snapshot() fileSnapshot {
	w.mu.Lock()
	path := w.path
	w.mu.Unlock()
	return w.snapshotPath(path)
}

func (w *manifestWatcher) snapshotPath(path string) fileSnapshot {
	fi, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fileSnapshot{exists: false, permOK: true}
		}
		if os.IsPermission(err) {
			return fileSnapshot{exists: true, permOK: false}
		}
		return fileSnapshot{exists: false, permOK: false}
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return fileSnapshot{exists: true, permOK: false}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsPermission(err) {
			return fileSnapshot{exists: true, permOK: false}
		}
		return fileSnapshot{exists: true, permOK: true, rawOK: false}
	}
	sum := sha256.Sum256(raw)
	rev := hex.EncodeToString(sum[:])
	return fileSnapshot{
		exists: true, permOK: true, rev: rev, rawOK: true,
		valid: looksLikeProjectYAML(raw),
	}
}

func looksLikeProjectYAML(raw []byte) bool {
	s := string(raw)
	return strings.Contains(s, "apiVersion:") && strings.Contains(s, "kind:") && strings.Contains(s, "Project")
}

func (w *manifestWatcher) run(ctxDone <-chan struct{}) {
	last := w.snapshot()
	w.mu.Lock()
	w.last = last
	w.mu.Unlock()
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctxDone:
			return
		case <-ticker.C:
			w.poll()
		}
	}
}

func (w *manifestWatcher) poll() {
	cur := w.snapshot()
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.pending != nil {
		if !sameSnapshot(*w.pending, cur) {
			cp := cur
			w.pending = &cp
			w.pendingSince = time.Now()
			return
		}
		if time.Since(w.pendingSince) < w.debounce {
			return
		}
		ev := w.classifyLocked(w.last, *w.pending)
		w.last = *w.pending
		w.pending = nil
		if ev != nil {
			w.hub.publish(*ev)
		}
		return
	}
	if sameSnapshot(w.last, cur) {
		return
	}
	cp := cur
	w.pending = &cp
	w.pendingSince = time.Now()
}

func sameSnapshot(a, b fileSnapshot) bool {
	return a.exists == b.exists && a.permOK == b.permOK && a.rev == b.rev && a.rawOK == b.rawOK && a.valid == b.valid
}

func (w *manifestWatcher) classifyLocked(prev, cur fileSnapshot) *ManifestEvent {
	if cur.exists && !cur.permOK {
		return &ManifestEvent{Kind: EventPermissionDenied, Revision: nonzero(cur.rev, "denied")}
	}
	if prev.exists && !cur.exists {
		return &ManifestEvent{Kind: EventDeleted, Revision: "missing"}
	}
	if !cur.exists {
		return nil
	}
	if !cur.rawOK {
		return &ManifestEvent{Kind: EventPermissionDenied, Revision: "unreadable"}
	}
	if !cur.valid {
		return &ManifestEvent{Kind: EventInvalid, Revision: cur.rev}
	}
	if prev.rev == cur.rev && prev.exists {
		return nil
	}
	if w.isSelfLocked(cur.rev) {
		return nil
	}
	return &ManifestEvent{Kind: EventChanged, Revision: cur.rev}
}

func nonzero(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
