package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"iacpack/internal/adapters/filesystem"
	"iacpack/internal/app/build"
	"iacpack/internal/diagnostic"
)

type buildStatus string

const (
	buildQueued    buildStatus = "queued"
	buildRunning   buildStatus = "running"
	buildSucceeded buildStatus = "succeeded"
	buildFailed    buildStatus = "failed"
	buildCancelled buildStatus = "cancelled"
)

type buildJob struct {
	ID          string
	Status      buildStatus
	Output      string
	Diagnostics []any
	cancel      context.CancelFunc
	events      []string
	mu          sync.Mutex
}

type buildQueue struct {
	max    int
	seq    atomic.Int64
	mu     sync.Mutex
	jobs   map[string]*buildJob
	active int
}

func newBuildQueue(max int) *buildQueue {
	return &buildQueue{max: max, jobs: map[string]*buildJob{}}
}

func (q *buildQueue) start(s *Server, format, outRel string) (*buildJob, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.active >= q.max {
		return nil, fmt.Errorf("build queue full")
	}
	id := fmt.Sprintf("build-%d", q.seq.Add(1))
	if strings.TrimSpace(outRel) == "" {
		outRel = filepath.ToSlash(filepath.Join(s.cfg.OutputRel, "builds", id))
	}
	ctx, cancel := context.WithCancel(context.Background())
	job := &buildJob{ID: id, Status: buildQueued, cancel: cancel}
	q.jobs[id] = job
	q.active++
	go s.runBuild(ctx, job, format, outRel)
	return job, nil
}

func (s *Server) runBuild(ctx context.Context, job *buildJob, format, outRel string) {
	defer func() {
		s.builds.mu.Lock()
		s.builds.active--
		s.builds.mu.Unlock()
	}()
	job.mu.Lock()
	job.Status = buildRunning
	job.pushLocked("status", "running")
	job.mu.Unlock()

	rel, err := containWorkspacePath(outRel)
	if err != nil {
		job.fail(diagnostic.Error(diagnostic.CodeCLIInvalidUsage, err.Error()))
		return
	}
	if !underOutputRoot(s.cfg.OutputRel, rel) {
		job.fail(diagnostic.Error(diagnostic.CodeCLIInvalidUsage, "build output must be under configured output root"))
		return
	}
	out := filepath.Join(s.workspace(), filepath.FromSlash(rel))
	if !underRoot(s.workspace(), out) {
		job.fail(diagnostic.Error(diagnostic.CodeCLIInvalidUsage, "build output escapes workspace"))
		return
	}
	select {
	case <-ctx.Done():
		job.cancelStatus()
		return
	default:
	}
	fs := filesystem.OS{}
	var res build.Result
	switch format {
	case "bundle":
		res = build.RunWithBundle(fs, build.OSWriter{}, build.OSWriter{}, s.manifestAbs(), build.Options{
			Policy: s.policy(), Workspace: mustWS(s.workspace()),
			Output: out, Format: format,
		})
	default:
		res = build.Run(fs, build.OSWriter{}, s.manifestAbs(), build.Options{
			Policy: s.policy(), Workspace: mustWS(s.workspace()),
			Output: out, Format: format,
		})
	}
	job.mu.Lock()
	defer job.mu.Unlock()
	if ctx.Err() != nil {
		job.Status = buildCancelled
		job.pushLocked("status", "cancelled")
		return
	}
	job.Diagnostics = diagnostic.ToMaps(res.Diagnostics)
	job.Output = rel
	if res.OK {
		job.Status = buildSucceeded
		job.pushLocked("status", "succeeded")
	} else {
		job.Status = buildFailed
		job.pushLocked("status", "failed")
	}
}

func underOutputRoot(outputRel, rel string) bool {
	outputRel = filepath.ToSlash(filepath.Clean(outputRel))
	rel = filepath.ToSlash(filepath.Clean(rel))
	return rel == outputRel || strings.HasPrefix(rel, outputRel+"/")
}

func (j *buildJob) fail(d diagnostic.Diagnostic) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Status = buildFailed
	j.Diagnostics = diagnostic.ToMaps([]diagnostic.Diagnostic{d})
	j.pushLocked("status", "failed")
}

func (j *buildJob) cancelStatus() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Status = buildCancelled
	j.pushLocked("status", "cancelled")
}

func (j *buildJob) pushLocked(event, data string) {
	const max = 1000
	line := fmt.Sprintf("event: %s\ndata: %s\n\n", event, data)
	j.events = append(j.events, line)
	if len(j.events) > max {
		j.events = j.events[len(j.events)-max:]
		j.events = append([]string{
			"event: build\ndata: {\"kind\":\"resyncRequired\",\"revision\":\"overflow\"}\n\n",
		}, j.events...)
	}
}

func (s *Server) handleStartBuild(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Format string `json:"format"`
		Output string `json:"output"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Format == "" {
		req.Format = "directory"
	}
	job, err := s.builds.start(s, req.Format, req.Output)
	if err != nil {
		writeProblem(w, http.StatusTooManyRequests, "Too Many Requests", err.Error(), nil)
		return
	}
	job.mu.Lock()
	status := job.Status
	id := job.ID
	job.mu.Unlock()
	writeJSON(w, http.StatusAccepted, map[string]any{"id": id, "status": status})
}

func (s *Server) handleGetBuild(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.builds.mu.Lock()
	job := s.builds.jobs[id]
	s.builds.mu.Unlock()
	if job == nil {
		writeProblem(w, http.StatusNotFound, "Not Found", "unknown build", nil)
		return
	}
	job.mu.Lock()
	defer job.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"id": job.ID, "status": job.Status, "output": job.Output, "diagnostics": job.Diagnostics,
	})
}

func (s *Server) handleCancelBuild(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.builds.mu.Lock()
	job := s.builds.jobs[id]
	s.builds.mu.Unlock()
	if job == nil {
		writeProblem(w, http.StatusNotFound, "Not Found", "unknown build", nil)
		return
	}
	if job.cancel != nil {
		job.cancel()
	}
	job.cancelStatus()
	writeJSON(w, http.StatusOK, map[string]any{"id": job.ID, "status": buildCancelled})
}

func (s *Server) handleBuildEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.builds.mu.Lock()
	job := s.builds.jobs[id]
	s.builds.mu.Unlock()
	if job == nil {
		writeProblem(w, http.StatusNotFound, "Not Found", "unknown build", nil)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Connection", "keep-alive")
	job.mu.Lock()
	events := append([]string(nil), job.events...)
	job.mu.Unlock()
	for _, e := range events {
		_, _ = w.Write([]byte(e))
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}
