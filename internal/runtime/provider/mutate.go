package provider

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"iacpack/internal/runtime/process"
)

// ApplyOps is the mutating Compose surface used by apply (P08).
type ApplyOps interface {
	ReadOnly
	EnsureNetwork(ctx context.Context, project, name string, labels map[string]string) error
	EnsureVolume(ctx context.Context, project, name string, labels map[string]string) error
	ImageAvailable(ctx context.Context, ref string) error
	ComposeUp(ctx context.Context, project, composeFile, service string, extraEnv []string) error
	WaitReady(ctx context.Context, project, service string, timeout time.Duration) error
	RunTask(ctx context.Context, project, composeFile, service string, timeout time.Duration) error
	OwnedIDs(ctx context.Context, project string) ([]string, error)
}

// RecordingApply extends Recording with mutation tracking for exact-order tests.
type RecordingApply struct {
	Recording
	Calls  []string
	FailAt string // action label to fail
}

func (r *RecordingApply) track(name string) error {
	r.Calls = append(r.Calls, name)
	r.Mutations++
	if r.FailAt != "" && r.FailAt == name {
		return fmt.Errorf("injected failure at %s", name)
	}
	return nil
}

func (r *RecordingApply) EnsureNetwork(ctx context.Context, project, name string, labels map[string]string) error {
	_ = ctx
	_ = project
	_ = labels
	return r.track("network:" + name)
}
func (r *RecordingApply) EnsureVolume(ctx context.Context, project, name string, labels map[string]string) error {
	_ = ctx
	_ = project
	_ = labels
	return r.track("volume:" + name)
}
func (r *RecordingApply) ImageAvailable(ctx context.Context, ref string) error {
	_ = ctx
	return r.track("image:" + ref)
}
func (r *RecordingApply) ComposeUp(ctx context.Context, project, composeFile, service string, extraEnv []string) error {
	_ = ctx
	_ = composeFile
	_ = extraEnv
	label := "up:" + project
	if service != "" {
		label += "/" + service
	}
	return r.track(label)
}
func (r *RecordingApply) WaitReady(ctx context.Context, project, service string, timeout time.Duration) error {
	_ = ctx
	_ = project
	_ = timeout
	return r.track("ready:" + service)
}
func (r *RecordingApply) RunTask(ctx context.Context, project, composeFile, service string, timeout time.Duration) error {
	_ = ctx
	_ = project
	_ = composeFile
	_ = timeout
	return r.track("task:" + service)
}
func (r *RecordingApply) OwnedIDs(ctx context.Context, project string) ([]string, error) {
	_ = ctx
	return []string{"compose:" + project}, nil
}

// HostApply mutates via docker compose argv.
type HostApply struct {
	Host
}

func (h HostApply) EnsureNetwork(ctx context.Context, project, name string, labels map[string]string) error {
	_ = project
	if name == "" {
		return fmt.Errorf("network name is empty")
	}
	bin, env, err := h.execBinEnv()
	if err != nil {
		return err
	}
	exists, err := h.networkExists(ctx, bin, env, name)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	args := []string{"network", "create"}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "--label", k+"="+labels[k])
	}
	args = append(args, name)
	res, err := h.runner().Run(ctx, process.Spec{Path: bin, Args: args, Env: env})
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("network create %s: %s", name, tailText(string(res.Stderr)+string(res.Stdout), 2000))
	}
	return nil
}

func (h HostApply) networkExists(ctx context.Context, bin string, env []string, name string) (bool, error) {
	res, err := h.runner().Run(ctx, process.Spec{Path: bin, Args: []string{"network", "exists", name}, Env: env})
	if err != nil {
		return false, err
	}
	if res.ExitCode == 0 {
		return true, nil
	}
	// Older engines have no `network exists`. inspect exit 0 means the network is there.
	res, err = h.runner().Run(ctx, process.Spec{Path: bin, Args: []string{"network", "inspect", name}, Env: env})
	if err != nil {
		return false, err
	}
	return res.ExitCode == 0, nil
}
func (h HostApply) EnsureVolume(ctx context.Context, project, name string, labels map[string]string) error {
	_ = ctx
	_ = project
	_ = name
	_ = labels
	return nil
}

// ImagePresent reports whether ref is already in the local engine. It does not pull.
func (h HostApply) ImagePresent(ctx context.Context, ref string) (bool, error) {
	if ref == "" || strings.HasPrefix(ref, "build:") {
		return false, nil
	}
	bin, env, err := h.execBinEnv()
	if err != nil {
		return false, err
	}
	res, err := h.runner().Run(ctx, process.Spec{Path: bin, Args: []string{"image", "inspect", ref}, Env: env})
	if err != nil {
		return false, err
	}
	return res.ExitCode == 0, nil
}

// ImageUserID resolves the image Config.User to a numeric uid and gid.
// An empty user or uid 0 means root inside the container; ok is false and no chown is needed.
func (h HostApply) ImageUserID(ctx context.Context, ref string) (uid, gid string, ok bool, err error) {
	if ref == "" || strings.HasPrefix(ref, "build:") {
		return "", "", false, nil
	}
	bin, env, err := h.execBinEnv()
	if err != nil {
		return "", "", false, err
	}
	res, err := h.runner().Run(ctx, process.Spec{
		Path: bin, Args: []string{"image", "inspect", "--format", "{{.Config.User}}", ref}, Env: env,
	})
	if err != nil {
		return "", "", false, err
	}
	if res.ExitCode != 0 {
		return "", "", false, fmt.Errorf("image inspect user %s: %s", ref, tailText(string(res.Stderr), 500))
	}
	user := strings.TrimSpace(string(res.Stdout))
	// uid 0 is container root. Rootless Podman already maps that to the host
	// user, and chown is neither required nor reported as a numeric owner.
	if user == "" || user == "0" || user == "0:0" {
		return "", "", false, nil
	}
	if u, g, numeric := numericUser(user); numeric {
		return u, g, true, nil
	}
	res, err = h.runner().Run(ctx, process.Spec{
		Path: bin,
		Args: []string{"run", "--rm", "--network", "none", "--user", user, "--entrypoint", "id", ref},
		Env:  env,
	})
	if err != nil {
		return "", "", false, err
	}
	if res.ExitCode != 0 {
		return "", "", false, fmt.Errorf("id for image user %s: %s", user, tailText(string(res.Stderr)+string(res.Stdout), 500))
	}
	u, g, parsed := parseID(string(res.Stdout))
	if !parsed {
		if strings.Contains(string(res.Stdout), "uid=0") {
			return "", "", false, nil
		}
		return "", "", false, fmt.Errorf("id for image user %s did not report a numeric uid", user)
	}
	return u, g, true, nil
}

// ImageWorkDir is the image Config.WorkingDir. An empty or relative value is
// ignored. Apply writes it into Compose so an entrypoint override still starts
// in that directory. Some Podman versions reset the working directory to /
// when --entrypoint is set, and a relative module such as uvicorn server:app
// then fails to import.
func (h HostApply) ImageWorkDir(ctx context.Context, ref string) (string, error) {
	if ref == "" || strings.HasPrefix(ref, "build:") {
		return "", nil
	}
	bin, env, err := h.execBinEnv()
	if err != nil {
		return "", err
	}
	res, err := h.runner().Run(ctx, process.Spec{
		Path: bin, Args: []string{"image", "inspect", "--format", "{{.Config.WorkingDir}}", ref}, Env: env,
	})
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		return "", fmt.Errorf("image inspect workdir %s: %s", ref, tailText(string(res.Stderr), 500))
	}
	return containerWorkDir(string(res.Stdout)), nil
}

func containerWorkDir(raw string) string {
	wd := strings.TrimSpace(raw)
	if !safeWorkDir(wd) {
		return ""
	}
	return wd
}

func safeWorkDir(wd string) bool {
	if wd == "" || strings.Contains(wd, "..") || !strings.HasPrefix(wd, "/") {
		return false
	}
	for _, r := range wd {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '/', r == '.', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

func parseID(s string) (uid, gid string, ok bool) {
	for _, f := range strings.Fields(s) {
		if strings.HasPrefix(f, "uid=") {
			uid = leadingDigits(strings.TrimPrefix(f, "uid="))
		}
		if strings.HasPrefix(f, "gid=") {
			gid = leadingDigits(strings.TrimPrefix(f, "gid="))
		}
	}
	if uid == "" || uid == "0" {
		return "", "", false
	}
	if gid == "" {
		gid = uid
	}
	return uid, gid, true
}

func leadingDigits(s string) string {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return s[:i]
}

// PullImages pulls every ref in one invocation. Each name is its own argument,
// never a comma-separated string.
func (h HostApply) PullImages(ctx context.Context, refs []string) error {
	if len(refs) == 0 {
		return nil
	}
	bin, env, err := h.execBinEnv()
	if err != nil {
		return err
	}
	args := make([]string, 0, 1+len(refs))
	args = append(args, "pull")
	args = append(args, refs...)
	res, err := h.runner().Run(ctx, process.Spec{Path: bin, Args: args, Env: env})
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("image pull failed: %s", tailText(string(res.Stderr)+string(res.Stdout), 2000))
	}
	return nil
}

func (h HostApply) ImageAvailable(ctx context.Context, ref string) error {
	bin, env, err := h.execBinEnv()
	if err != nil {
		return err
	}
	res, err := h.runner().Run(ctx, process.Spec{Path: bin, Args: []string{"image", "inspect", ref}, Env: env})
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		res, err = h.runner().Run(ctx, process.Spec{Path: bin, Args: []string{"pull", ref}, Env: env})
		if err != nil {
			return err
		}
		if res.ExitCode != 0 {
			return fmt.Errorf("image pull failed: %s", tailText(string(res.Stderr)+string(res.Stdout), 2000))
		}
	}
	return nil
}
func (h HostApply) ComposeUp(ctx context.Context, project, composeFile, service string, extraEnv []string) error {
	return h.composeUp(ctx, project, composeFile, service, extraEnv, false)
}

// ComposeUpRecreate is ComposeUp with --force-recreate for one service.
// A container left Created after a CDI failure is replaced.
func (h HostApply) ComposeUpRecreate(ctx context.Context, project, composeFile, service string, extraEnv []string) error {
	return h.composeUp(ctx, project, composeFile, service, extraEnv, true)
}

func (h HostApply) composeUp(ctx context.Context, project, composeFile, service string, extraEnv []string, recreate bool) error {
	bin, env, err := h.execBinEnv()
	if err != nil {
		return err
	}
	if len(extraEnv) > 0 {
		env = append(append([]string{}, env...), extraEnv...)
	}
	// Avoid --remove-orphans=false: Docker Compose V2 accepts it, but
	// podman-compose rejects boolean "=false" and dumps usage (E2E stand-in).
	// Orphan policy is COMPOSE_IGNORE_ORPHANS=true in env().
	// One service per call so a single create failure is not the whole project.
	// Always --force-recreate: a prior apply may have left Created/Exited
	// containers whose HostConfig still has read-only binds from an older compose.
	args := []string{"compose", "-p", project, "-f", composeFile, "up", "-d", "--no-deps", "--force-recreate"}
	_ = recreate
	if service != "" {
		args = append(args, service)
	}
	res, err := h.runner().Run(ctx, process.Spec{
		Path: bin,
		Args: args,
		Env:  env,
	})
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return composeUpError(service, string(res.Stderr), string(res.Stdout))
	}
	return nil
}

// CDIUnavailableError is a compose-up failure naming one CDI device.
type CDIUnavailableError struct {
	Service string
	Device  string
}

func (e *CDIUnavailableError) Error() string {
	if e.Service != "" {
		return fmt.Sprintf("service %q: CDI device %q is not available", e.Service, e.Device)
	}
	return fmt.Sprintf("CDI device %q is not available", e.Device)
}

func composeUpError(service, stderr, stdout string) error {
	blob := stderr + "\n" + stdout
	if dev := cdiDeviceName(blob); dev != "" {
		return &CDIUnavailableError{Service: service, Device: dev}
	}
	detail := composeFailureDetail(stderr)
	if detail == "" {
		detail = composeFailureDetail(stdout)
	}
	if detail == "" {
		detail = "no engine output"
	}
	if service != "" {
		return fmt.Errorf("service %q: compose up failed: %s", service, detail)
	}
	return fmt.Errorf("compose up failed: %s", detail)
}

func cdiDeviceName(blob string) string {
	const marker = "unresolvable CDI devices"
	i := strings.Index(blob, marker)
	if i < 0 {
		return ""
	}
	rest := strings.TrimSpace(blob[i+len(marker):])
	rest = strings.TrimLeft(rest, ":")
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return ""
	}
	if j := strings.IndexAny(rest, " \t\r\n,;"); j >= 0 {
		rest = rest[:j]
	}
	return strings.Trim(rest, `"'`)
}

func composeFailureDetail(blob string) string {
	var lines []string
	for _, ln := range strings.Split(blob, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		lines = append(lines, ln)
	}
	for _, ln := range lines {
		low := strings.ToLower(ln)
		if strings.Contains(ln, "unresolvable CDI") || strings.Contains(low, "error") {
			return redact(ln)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return redact(lines[len(lines)-1])
}

// OwnWritable chowns writable bind directories and named volumes as the
// service uid inside the Podman user namespace. Rootless containers otherwise
// see the host user as root and a process at uid 1000 cannot write /logs.
// Docker is unchanged: host uids are already the container uids.
func (h HostApply) OwnWritable(ctx context.Context, project, user string, binds, volumes []string) error {
	uid, gid, numeric := numericUser(user)
	eng, err := h.resolvedEngine()
	if err != nil || eng != EnginePodman {
		return nil
	}
	bin, env, err := h.execBinEnv()
	if err != nil {
		return err
	}
	for _, vol := range volumes {
		name := project + "_" + vol
		_, _ = h.runner().Run(ctx, process.Spec{Path: bin, Args: []string{"volume", "create", name}, Env: env})
		res, err := h.runner().Run(ctx, process.Spec{
			Path: bin, Args: []string{"volume", "inspect", "--format", "{{.Mountpoint}}", name}, Env: env,
		})
		if err != nil || res.ExitCode != 0 {
			return fmt.Errorf("volume %s mountpoint: %s", name, strings.TrimSpace(string(res.Stderr)))
		}
		mp := strings.TrimSpace(string(res.Stdout))
		if numeric {
			if err := h.unshareChown(ctx, bin, env, uid, gid, mp); err != nil {
				return err
			}
		} else if err := h.unshareChmod(ctx, bin, env, mp); err != nil {
			return err
		}
	}
	for _, dir := range binds {
		if !ownablePath(dir) {
			continue
		}
		if !numeric && !payloadBind(dir) {
			continue
		}
		st, err := os.Stat(dir)
		if err != nil {
			if !os.IsNotExist(err) {
				return err
			}
			// Empty payload resources (logs, data) are not files in the bundle.
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
		}
		_ = st
		if numeric {
			if err := h.unshareChown(ctx, bin, env, uid, gid, dir); err != nil {
				return err
			}
			continue
		}
		// Images such as library postgres leave Config.User empty, start as
		// root, then drop to uid 999 before opening /logs. A host-owned 0755
		// directory is not writable by that uid. Mode 0777 on payload binds
		// lets that process create its log file.
		if err := h.unshareChmod(ctx, bin, env, dir); err != nil {
			return err
		}
	}
	return nil
}

func payloadBind(path string) bool {
	return strings.Contains(filepath.ToSlash(path), "/payload/")
}

func (h HostApply) unshareChmod(ctx context.Context, bin string, env []string, path string) error {
	res, err := h.runner().Run(ctx, process.Spec{
		Path: bin,
		Args: []string{"unshare", "chmod", "0777", path},
		Env:  env,
	})
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		msg := strings.TrimSpace(string(res.Stderr))
		if msg == "" {
			msg = strings.TrimSpace(string(res.Stdout))
		}
		return fmt.Errorf("podman unshare chmod 0777 %s: %s", path, msg)
	}
	return nil
}

func (h HostApply) unshareChown(ctx context.Context, bin string, env []string, uid, gid, path string) error {
	res, err := h.runner().Run(ctx, process.Spec{
		Path: bin,
		Args: []string{"unshare", "chown", "-R", uid + ":" + gid, path},
		Env:  env,
	})
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		msg := strings.TrimSpace(string(res.Stderr))
		if msg == "" {
			msg = strings.TrimSpace(string(res.Stdout))
		}
		return fmt.Errorf("podman unshare chown %s:%s %s: %s", uid, gid, path, msg)
	}
	return nil
}

func numericUser(user string) (uid, gid string, ok bool) {
	user = strings.TrimSpace(user)
	if user == "" || user == "0" || user == "0:0" {
		return "", "", false
	}
	parts := strings.Split(user, ":")
	if len(parts) == 1 {
		if !digitsOnly(parts[0]) {
			return "", "", false
		}
		return parts[0], parts[0], true
	}
	if len(parts) == 2 && digitsOnly(parts[0]) && digitsOnly(parts[1]) && parts[0] != "0" {
		return parts[0], parts[1], true
	}
	return "", "", false
}

func digitsOnly(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func ownablePath(p string) bool {
	c := filepath.Clean(p)
	return filepath.IsAbs(c) && c != string(filepath.Separator)
}

// ServiceLogs returns the last lines of a service container. Used when a
// health check fails so the operator sees why the process is unhealthy.
func (h HostApply) ServiceLogs(ctx context.Context, project, service string, lines int) (string, error) {
	if lines <= 0 {
		lines = 80
	}
	bin, env, err := h.execBinEnv()
	if err != nil {
		return "", err
	}
	id, err := h.composeServiceContainerID(ctx, bin, env, project, service, false)
	if err != nil || id == "" {
		return "", err
	}
	res, err := h.runner().Run(ctx, process.Spec{
		Path: bin,
		Args: []string{"logs", "--tail", strconv.Itoa(lines), id},
		Env:  env,
	})
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(string(res.Stdout) + string(res.Stderr))
	return lastLines(text, lines), nil
}

func lastLines(text string, n int) string {
	if text == "" || n <= 0 {
		return ""
	}
	lines := strings.Split(text, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func (h HostApply) WaitReady(ctx context.Context, project, service string, timeout time.Duration) error {
	bin, env, err := h.execBinEnv()
	if err != nil {
		return err
	}
	deadline := time.Now().Add(timeout)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		ready, _ := h.composeServiceReady(ctx, bin, env, project, service)
		if ready {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("readiness timeout for %s", service)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// EnsurePostgresDatabase creates db owned by user when the cluster is already
// initialized and that database is missing. Maintenance DB is postgres, then
// template1. Identifiers are restricted to unquoted Postgres names.
func (h HostApply) EnsurePostgresDatabase(ctx context.Context, project, service, user, db string) error {
	if !pgName(user) || !pgName(db) {
		return fmt.Errorf("refused postgres identity")
	}
	bin, env, err := h.execBinEnv()
	if err != nil {
		return err
	}
	id, err := h.composeServiceContainerID(ctx, bin, env, project, service, true)
	if err != nil {
		return err
	}
	if id == "" {
		return fmt.Errorf("no running container")
	}
	sql := "DO $$BEGIN IF NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = " + sqlQuote(db) +
		") THEN EXECUTE format('CREATE DATABASE %I OWNER %I', " + sqlQuote(db) + ", " + sqlQuote(user) + "); END IF; END$$;"
	var last error
	for _, maint := range []string{"postgres", "template1"} {
		res, err := h.runner().Run(ctx, process.Spec{
			Path: bin,
			Args: []string{"exec", id, "psql", "-U", user, "-d", maint, "--variable=ON_ERROR_STOP=1", "--command=" + sql},
			Env:  env,
		})
		if err != nil {
			return err
		}
		if res.ExitCode == 0 {
			return nil
		}
		last = fmt.Errorf("%s", tailText(string(res.Stderr)+string(res.Stdout), 500))
	}
	if last == nil {
		last = fmt.Errorf("psql failed")
	}
	return last
}

func pgName(s string) bool {
	if s == "" || len(s) > 63 {
		return false
	}
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}

func sqlQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// composeServiceReady uses engine `ps`/`inspect` labels. podman-compose 1.6
// rejects `compose ps --status running --services` (Docker Compose V2 flags).
func (h HostApply) composeServiceReady(ctx context.Context, bin string, env []string, project, service string) (bool, error) {
	id, err := h.composeServiceContainerID(ctx, bin, env, project, service, true)
	if err != nil || id == "" {
		return false, err
	}
	res, err := h.runner().Run(ctx, process.Spec{
		Path: bin,
		Args: []string{"inspect", "--format",
			`{{.State.Status}} {{if .State.Health}}{{.State.Health.Status}}{{else}}-{{end}}`, id},
		Env: env,
	})
	if err != nil || res.ExitCode != 0 {
		return false, err
	}
	fields := strings.Fields(strings.TrimSpace(string(res.Stdout)))
	if len(fields) == 0 {
		return false, nil
	}
	status := fields[0]
	if status != "running" {
		return false, nil
	}
	if len(fields) < 2 || fields[1] == "-" || fields[1] == "" {
		return true, nil
	}
	return fields[1] == "healthy", nil
}

func (h HostApply) composeServiceContainerID(
	ctx context.Context, bin string, env []string, project, service string, runningOnly bool,
) (string, error) {
	labelSets := [][2]string{
		{"com.docker.compose.project", "com.docker.compose.service"},
		{"io.podman.compose.project", "io.podman.compose.service"},
	}
	for _, labels := range labelSets {
		args := []string{
			"ps", "-aq",
			"--filter", "label=" + labels[0] + "=" + project,
			"--filter", "label=" + labels[1] + "=" + service,
		}
		if runningOnly {
			args = append(args, "--filter", "status=running")
		}
		res, err := h.runner().Run(ctx, process.Spec{Path: bin, Args: args, Env: env})
		if err != nil || res.ExitCode != 0 {
			continue
		}
		var id string
		for _, line := range strings.Split(string(res.Stdout), "\n") {
			id = strings.TrimSpace(line)
			if id != "" {
				// podman ps lists the newest container first.
				return id, nil
			}
		}
	}
	return "", nil
}

// MountReadOnlyError is a bind that should be writable but is mounted read-only.
type MountReadOnlyError struct {
	Service string
	Target  string
}

func (e *MountReadOnlyError) Error() string {
	return fmt.Sprintf("service %q: mount %q is read-only", e.Service, e.Target)
}

// AssertWritableMounts fails when a path that must be writable is mounted RO.
func (h HostApply) AssertWritableMounts(ctx context.Context, project, service string, targets []string) error {
	if len(targets) == 0 {
		return nil
	}
	bin, env, err := h.execBinEnv()
	if err != nil {
		return err
	}
	id, err := h.composeServiceContainerID(ctx, bin, env, project, service, false)
	if err != nil {
		return err
	}
	if id == "" {
		return nil
	}
	res, err := h.runner().Run(ctx, process.Spec{
		Path: bin,
		Args: []string{"inspect", "--format",
			`{{range .Mounts}}{{.Destination}} {{.RW}}{{"\n"}}{{end}}`, id},
		Env: env,
	})
	if err != nil || res.ExitCode != 0 {
		return err
	}
	rw := map[string]bool{}
	for _, line := range strings.Split(string(res.Stdout), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		rw[fields[0]] = strings.EqualFold(fields[1], "true")
	}
	for _, tgt := range targets {
		if ok, seen := rw[tgt]; seen && !ok {
			return &MountReadOnlyError{Service: service, Target: tgt}
		}
	}
	return nil
}
func (h HostApply) RunTask(ctx context.Context, project, composeFile, service string, timeout time.Duration) error {
	bin, env, err := h.execBinEnv()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	// Tasks are rendered with profiles: [iacpack-task]. podman-compose 1.6 and
	// Docker Compose omit them from `run` unless --profile is set (global flag).
	res, err := h.runner().Run(ctx, process.Spec{
		Path:   bin,
		Args:   []string{"compose", "--profile", "iacpack-task", "-p", project, "-f", composeFile, "run", "-T", "--rm", "--no-deps", service},
		Env:    env,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	})
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("task %s failed: %s", service, tailText(string(res.Stderr)+string(res.Stdout), 2000))
	}
	return nil
}
func (h HostApply) OwnedIDs(ctx context.Context, project string) ([]string, error) {
	_ = ctx
	return []string{"compose:" + project}, nil
}

func (h Host) execBinEnv() (bin string, env []string, err error) {
	env, err = h.runtimeEnv()
	if err != nil {
		return "", nil, err
	}
	bin, err = h.cliBin()
	if err != nil {
		return "", nil, err
	}
	return bin, env, nil
}

func redact(s string) string {
	if len(s) > 512 {
		return s[:512]
	}
	return strings.TrimSpace(s)
}

func tailText(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

// HostDestroy mutates via docker compose down / volume rm (P09).
type HostDestroy struct {
	Host
}

func (h HostDestroy) Down(ctx context.Context, project, composeFile string) error {
	bin, env, err := h.execBinEnv()
	if err != nil {
		return err
	}
	args := []string{"compose", "-p", project}
	if composeFile != "" {
		args = append(args, "-f", composeFile)
	}
	// No --volumes: retain named volumes by default (TZ destroy retention).
	args = append(args, "down")
	res, err := h.runner().Run(ctx, process.Spec{Path: bin, Args: args, Env: env})
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("compose down failed: %s", redact(string(res.Stderr)))
	}
	return nil
}

func (h HostDestroy) DeleteManagedVolumes(ctx context.Context, project string, names []string) error {
	_ = project
	bin, env, err := h.execBinEnv()
	if err != nil {
		return err
	}
	for _, name := range names {
		res, err := h.runner().Run(ctx, process.Spec{
			Path: bin,
			Args: []string{"volume", "rm", name},
			Env:  env,
		})
		if err != nil {
			return err
		}
		if res.ExitCode != 0 {
			return fmt.Errorf("volume rm %s failed: %s", name, redact(string(res.Stderr)))
		}
	}
	return nil
}

func (h HostDestroy) ListOwned(ctx context.Context, project string) ([]string, error) {
	_ = ctx
	return []string{"compose:" + project}, nil
}

// MaterializeSecrets writes secret files 0440 under secretsDir 0700.
// Apply later chowns them to the service/task uid so a non-root user can read.
func MaterializeSecrets(secretsDir string, files map[string]string) error {
	if err := os.MkdirAll(secretsDir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(secretsDir, 0o700); err != nil {
		return err
	}
	for id, src := range files {
		data, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("secret %s unreadable", id)
		}
		dest := filepath.Join(secretsDir, id)
		tmp := dest + ".tmp"
		if err := os.WriteFile(tmp, data, 0o440); err != nil {
			return err
		}
		if err := os.Chmod(tmp, 0o440); err != nil {
			return err
		}
		if err := os.Rename(tmp, dest); err != nil {
			return err
		}
		for i := range data {
			data[i] = 0
		}
	}
	return nil
}

// OwnSecrets makes secret files readable by the container uid (mode 0440).
// Rootless Podman otherwise sees them as root-owned 0400 and uid 1000 fails.
func (h HostApply) OwnSecrets(ctx context.Context, user string, paths []string) error {
	uid, gid, ok := numericUser(user)
	if !ok {
		return nil
	}
	eng, err := h.resolvedEngine()
	if err != nil || eng != EnginePodman {
		for _, p := range paths {
			if err := os.Chmod(p, 0o440); err != nil {
				return err
			}
		}
		return nil
	}
	bin, env, err := h.execBinEnv()
	if err != nil {
		return err
	}
	for _, p := range paths {
		if !ownablePath(p) {
			continue
		}
		if err := h.unshareChown(ctx, bin, env, uid, gid, p); err != nil {
			return err
		}
		res, err := h.runner().Run(ctx, process.Spec{
			Path: bin, Args: []string{"unshare", "chmod", "0440", p}, Env: env,
		})
		if err != nil {
			return err
		}
		if res.ExitCode != 0 {
			return fmt.Errorf("podman unshare chmod 0440 %s: %s", p, strings.TrimSpace(string(res.Stderr)))
		}
	}
	return nil
}
