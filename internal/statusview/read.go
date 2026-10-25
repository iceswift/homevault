// Package statusview reads bounded local metadata. It never creates directories,
// takes write locks, opens credential files, runs commands or contacts services.
package statusview

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"homevault/internal/statusdata"
)

const limit = 1024 * 1024

var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)
var idPattern = regexp.MustCompile(`^[0-9a-f]{24}$`)
var missing = errors.New("record-missing")

func code(value string) error { return errors.New(value) }

// regularDocument rejects links (including parents), special files and races.
// Errors contain fixed codes only, never paths, JSON fragments or private values.
func regularDocument(path string) (map[string]any, error) {
	if !filepath.IsAbs(path) || strings.HasPrefix(path, `\\`) || strings.HasPrefix(path, "//") {
		return nil, code("unsafe-path")
	}
	path = filepath.Clean(path)
	for parent := filepath.Dir(path); ; parent = filepath.Dir(parent) {
		info, err := os.Lstat(parent)
		if os.IsNotExist(err) {
			return nil, missing
		}
		if err != nil {
			return nil, code("unreadable-record")
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, code("unsafe-path")
		}
		if filepath.Dir(parent) == parent {
			break
		}
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, missing
	}
	if err != nil {
		return nil, code("unreadable-record")
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > limit {
		return nil, code("unsafe-record")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, code("unreadable-record")
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) || !opened.Mode().IsRegular() {
		return nil, code("record-changed-during-read")
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || len(data) > limit || !utf8.Valid(data) {
		return nil, code("unreadable-record")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	nodes := 0
	value, err := decode(decoder, 0, &nodes)
	if err != nil {
		return nil, code("invalid-json")
	}
	if _, err = decoder.Token(); err != io.EOF {
		return nil, code("invalid-json")
	}
	result, ok := value.(map[string]any)
	if !ok {
		return nil, code("invalid-json")
	}
	return result, nil
}
func decode(d *json.Decoder, depth int, nodes *int) (any, error) {
	*nodes++
	if depth > 64 || *nodes > 32768 {
		return nil, code("json-limit")
	}
	token, err := d.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return token, nil
	}
	switch delim {
	case '{':
		result := map[string]any{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return nil, err
			}
			s, ok := key.(string)
			if !ok {
				return nil, code("key")
			}
			if _, exists := result[s]; exists {
				return nil, code("duplicate-key")
			}
			value, err := decode(d, depth+1, nodes)
			if err != nil {
				return nil, err
			}
			result[s] = value
		}
		end, err := d.Token()
		if err != nil || end != json.Delim('}') {
			return nil, code("object")
		}
		return result, nil
	case '[':
		result := []any{}
		for d.More() {
			if len(result) >= 4096 {
				return nil, code("array-limit")
			}
			value, err := decode(d, depth+1, nodes)
			if err != nil {
				return nil, err
			}
			result = append(result, value)
		}
		end, err := d.Token()
		if err != nil || end != json.Delim(']') {
			return nil, code("array")
		}
		return result, nil
	}
	return nil, code("delimiter")
}
func text(m map[string]any, key string) string           { s, _ := m[key].(string); return s }
func object(m map[string]any, key string) map[string]any { s, _ := m[key].(map[string]any); return s }
func flag(m map[string]any, key string) (bool, bool)     { v, ok := m[key].(bool); return v, ok }
func integer(v any) (int, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	i, err := strconv.Atoi(string(n))
	return i, err == nil
}
func version(m map[string]any) bool { n, ok := integer(m["version"]); return ok && n == 1 }
func keys(m map[string]any, allowed ...string) bool {
	if len(m) != len(allowed) {
		return false
	}
	for _, key := range allowed {
		if _, ok := m[key]; !ok {
			return false
		}
	}
	return true
}
func hash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// pythonJSON matches json.dumps(sort_keys=True, ensure_ascii=True) for the
// integer-only workflow schema. Unsupported floating policies never become green.
func pythonJSON(value any) (string, error) {
	switch v := value.(type) {
	case nil:
		return "null", nil
	case bool:
		if v {
			return "true", nil
		}
		return "false", nil
	case string:
		var b strings.Builder
		b.WriteByte('"')
		for _, r := range v {
			switch r {
			case '"', '\\':
				b.WriteByte('\\')
				b.WriteRune(r)
			case '\b':
				b.WriteString(`\b`)
			case '\f':
				b.WriteString(`\f`)
			case '\n':
				b.WriteString(`\n`)
			case '\r':
				b.WriteString(`\r`)
			case '\t':
				b.WriteString(`\t`)
			default:
				if r < 32 || r >= 127 {
					if r > 0xffff {
						a, c := utf16.EncodeRune(r)
						fmt.Fprintf(&b, `\u%04x\u%04x`, a, c)
					} else {
						fmt.Fprintf(&b, `\u%04x`, r)
					}
				} else {
					b.WriteRune(r)
				}
			}
		}
		b.WriteByte('"')
		return b.String(), nil
	case json.Number:
		raw := string(v)
		if len(raw) > 128 || strings.ContainsAny(raw, ".eE") {
			return "", code("unsupported-policy-number")
		}
		n, ok := new(big.Int).SetString(raw, 10)
		if !ok {
			return "", code("policy-number")
		}
		return n.String(), nil
	case []any:
		values := []string{}
		for _, item := range v {
			s, err := pythonJSON(item)
			if err != nil {
				return "", err
			}
			values = append(values, s)
		}
		return "[" + strings.Join(values, ", ") + "]", nil
	case map[string]any:
		names := []string{}
		for name := range v {
			names = append(names, name)
		}
		sort.Strings(names)
		values := []string{}
		for _, name := range names {
			k, _ := pythonJSON(name)
			s, err := pythonJSON(v[name])
			if err != nil {
				return "", err
			}
			values = append(values, k+": "+s)
		}
		return "{" + strings.Join(values, ", ") + "}", nil
	}
	return "", code("unsupported-policy-value")
}
func binding(workflow map[string]any, repoID string) (string, error) {
	s, err := pythonJSON(map[string]any{"workflow": workflow, "repo": repoID})
	if err != nil {
		return "", err
	}
	return hash(s), nil
}
func stamp(m map[string]any, key string) (time.Time, error) {
	value, exists := m[key]
	if !exists {
		return time.Time{}, nil
	}
	s, ok := value.(string)
	if !ok || s == "" {
		return time.Time{}, code("invalid-time")
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, code("invalid-time")
	}
	return t.UTC(), nil
}
func validTime(t, at time.Time) bool { return t.IsZero() || !t.After(at.Add(5*time.Minute)) }
func kindName(kind string) string {
	return map[string]string{"sqlite": "SQLite capture", "sealed": "Completed export bundle", "json": "JSON-state capture", "files": "Selected files"}[kind]
}
func unknown(row statusdata.Project, reason string) statusdata.Project {
	row.State = "unknown"
	row.Attempt = "Unknown - local record not trusted"
	row.Captured = time.Time{}
	row.Uploaded = time.Time{}
	row.RecordScope = "Unvalidated local record"
	row.Note = "Local metadata unavailable or unsupported (" + reason + "). No live repository or service was checked."
	return row
}

// Read observes plan gates plus coordinator records, with explicitly labelled
// legacy engine-job context. It never equates a missing record with no backup.
func Read(planPath string, at time.Time) (statusdata.Snapshot, error) {
	return readSnapshot(planPath, at, regularDocument)
}

func readSnapshot(planPath string, at time.Time, document func(string) (map[string]any, error)) (statusdata.Snapshot, error) {
	result := statusdata.Snapshot{ReadAt: at.UTC()}
	fail := func() (statusdata.Snapshot, error) {
		return statusdata.Snapshot{}, code("read-only plan unavailable or unsupported; check its path, format and read permissions")
	}
	if at.IsZero() {
		return fail()
	}
	plan, err := document(planPath)
	if err != nil {
		return fail()
	}
	if !keys(plan, "version", "root", "base_config", "homevault_binary", "notifications_enabled", "workflows") || !version(plan) {
		return fail()
	}
	notify, ok := flag(plan, "notifications_enabled")
	if !ok {
		return fail()
	}
	result.Notifications = "Disabled in plan"
	if notify {
		result.Notifications = "Enabled in plan; this view never sends"
	}
	root, basePath := text(plan, "root"), text(plan, "base_config")
	if !filepath.IsAbs(root) || !filepath.IsAbs(basePath) || !filepath.IsAbs(text(plan, "homevault_binary")) {
		return fail()
	}
	base, err := document(basePath)
	if err != nil || !version(base) {
		return fail()
	}
	repository, rclonePath, stateDir := text(base, "repository"), text(base, "rclone_config_file"), text(base, "state_dir")
	if repository == "" || strings.ContainsAny(repository, "\x00\r\n") || !filepath.IsAbs(stateDir) || (rclonePath != "" && !filepath.IsAbs(rclonePath)) {
		return fail()
	}
	repoID := hash(repository + "\x00" + rclonePath)
	legacyIdentity := repository
	if strings.HasPrefix(repository, "rclone:") && rclonePath != "" {
		legacyIdentity += "\x00rclone-config\x00" + rclonePath
	}
	legacyID := hash(legacyIdentity)
	workflows, ok := plan["workflows"].([]any)
	if !ok || len(workflows) == 0 || len(workflows) > 128 {
		return fail()
	}
	names, projects := map[string]bool{}, map[string]bool{}
	for _, value := range workflows {
		w, ok := value.(map[string]any)
		if !ok {
			return fail()
		}
		name, project, kind := text(w, "name"), text(w, "project"), text(w, "kind")
		enabled, flagOK := flag(w, "enabled")
		maxAge, ageOK := integer(w["max_age_hours"])
		allowed := []string{"name", "project", "kind", "enabled", "max_age_hours"}
		switch kind {
		case "sqlite":
			allowed = append(allowed, "capture_plan")
		case "sealed":
			allowed = append(allowed, "source_root", "source_lock", "deployment_files", "image_pins")
		case "json":
			allowed = append(allowed, "container_id", "image_id", "mount_source", "mount_target", "files", "window", "max_pause_seconds")
		case "files":
			allowed = append(allowed, "files", "trees")
		default:
			return fail()
		}
		if !keys(w, allowed...) || !namePattern.MatchString(name) || !namePattern.MatchString(project) || names[name] || projects[project] || !flagOK || !ageOK || maxAge < 1 || maxAge > 720 {
			return fail()
		}
		names[name] = true
		projects[project] = true
		row := statusdata.Project{ID: name, Name: name, ProjectName: project, Kind: kindName(kind), State: "unverified", Attempt: "No local record", Recovery: "Unknown - not inspected", Schedule: "Unknown - systemd not inspected", Activation: "Disabled in plan", RecordScope: "No coordinator record", Note: "No record in this metadata set does not mean no cloud backup exists. Recovery, cloud reachability and timers were not inspected."}
		if enabled {
			row.Activation = "Enabled in plan; timer unknown"
		} else {
			row.State = "inactive"
		}
		record, readErr := document(filepath.Join(root, "state", name+".json"))
		switch {
		case errors.Is(readErr, missing):
			legacy, legacyErr := document(filepath.Join(stateDir, "jobs", project+".json"))
			if errors.Is(legacyErr, missing) {
				break
			}
			if legacyErr != nil {
				row = unknown(row, "legacy-record-unreadable")
				break
			}
			upload, attempt, err := legacyJob(legacy, project, legacyID, at)
			if err != nil {
				row = unknown(row, "legacy-identity-or-format")
				break
			}
			row.Uploaded = upload
			row.Attempt = attempt + " (legacy job)"
			row.RecordScope = "Legacy engine job only"
			row.Note = "Earlier CLI job context only. Source capture age is unknown; it cannot establish workflow freshness. Other snapshots may exist."
		case readErr != nil:
			row = unknown(row, "coordinator-record-unreadable")
		default:
			expected, err := binding(w, repoID)
			if err != nil || !version(record) || text(record, "binding") != expected {
				row = unknown(row, "policy-or-repository-mismatch")
				break
			}
			row, err = coordinator(row, record, enabled, time.Duration(maxAge)*time.Hour, at)
			if err != nil {
				row = unknown(row, "invalid-coordinator-record")
			}
		}
		result.Projects = append(result.Projects, row)
	}
	planAfter, e1 := document(planPath)
	baseAfter, e2 := document(basePath)
	if e1 != nil || e2 != nil || !reflect.DeepEqual(plan, planAfter) || !reflect.DeepEqual(base, baseAfter) {
		return fail()
	}
	return result, nil
}

func legacyJob(record map[string]any, project, identity string, at time.Time) (time.Time, string, error) {
	fail := func() (time.Time, string, error) { return time.Time{}, "", code("invalid-legacy-record") }
	if !version(record) || text(record, "project") != project || text(record, "repository_id") != identity {
		return fail()
	}
	latest := object(record, "latest")
	history, ok := record["history"].([]any)
	if latest == nil || !ok || len(history) > 32 {
		return fail()
	}
	upload, err := stamp(record, "last_success")
	if err != nil || !validTime(upload, at) {
		return fail()
	}
	validate := func(a map[string]any) bool {
		if !idPattern.MatchString(text(a, "id")) {
			return false
		}
		started, e1 := stamp(a, "started")
		finished, e2 := stamp(a, "finished")
		if e1 != nil || e2 != nil || started.IsZero() || !validTime(started, at) || !validTime(finished, at) {
			return false
		}
		phase, status := text(a, "phase"), text(a, "status")
		if status == "running" {
			return phase == "capture" || phase == "backup" || phase == "check"
		}
		if finished.IsZero() || finished.Before(started) {
			return false
		}
		if status == "success" {
			b, bok := flag(a, "backup_passed")
			c, cok := flag(a, "check_passed")
			return phase == "completed" && bok && cok && b && c
		}
		return status == "failed" && (phase == "capture" || phase == "backup" || phase == "check" || phase == "cancelled")
	}
	if !validate(latest) {
		return fail()
	}
	for _, entry := range history {
		a, ok := entry.(map[string]any)
		if !ok || !validate(a) {
			return fail()
		}
	}
	status := text(latest, "status")
	if status == "success" {
		finished, _ := stamp(latest, "finished")
		if !upload.Equal(finished) {
			return fail()
		}
	}
	label := map[string]string{"success": "Backup + check passed", "failed": "Failed attempt", "running": "Running record; process not checked"}[status]
	return upload, label, nil
}

func coordinator(row statusdata.Project, record map[string]any, enabled bool, maxAge time.Duration, at time.Time) (statusdata.Project, error) {
	fail := func() (statusdata.Project, error) { return row, code("invalid-state") }
	latest := object(record, "latest")
	history, ok := record["history"].([]any)
	if latest == nil || !ok || len(history) > 32 {
		return fail()
	}
	captured, e1 := stamp(record, "captured_at")
	uploaded, e2 := stamp(record, "last_success")
	if e1 != nil || e2 != nil || captured.IsZero() != uploaded.IsZero() || !validTime(captured, at) || !validTime(uploaded, at) || captured.After(uploaded.Add(5*time.Minute)) {
		return fail()
	}
	status := text(latest, "result")
	if status == "" && (!uploaded.IsZero() || len(history) > 0) {
		return fail()
	}
	if status != "" && status != "success" && status != "failed" && status != "running" && status != "skipped" {
		return fail()
	}
	started, e := stamp(latest, "started")
	if e != nil || !validTime(started, at) || status != "" && (started.IsZero() || !idPattern.MatchString(text(latest, "id"))) {
		return fail()
	}
	if !uploaded.IsZero() {
		proof := false
		candidates := append(append([]any{}, history...), latest)
		for _, entry := range candidates {
			a, ok := entry.(map[string]any)
			if !ok {
				return fail()
			}
			if text(a, "result") != "success" {
				continue
			}
			c, ce := stamp(a, "captured_at")
			finished, fe := stamp(a, "finished")
			began, be := stamp(a, "started")
			cv, cok := flag(a, "capture_verified")
			uv, uok := flag(a, "upload_verified")
			check, vok := flag(a, "check_verified")
			if text(a, "phase") != "completed" || ce != nil || fe != nil || be != nil || !idPattern.MatchString(text(a, "id")) || began.IsZero() || finished.IsZero() || finished.Before(began) || !validTime(finished, at) || !cok || !uok || !vok || !cv || !uv || !check {
				return fail()
			}
			if c.Equal(captured) && !finished.Before(uploaded) && uploaded.After(began.Add(-5*time.Minute)) {
				proof = true
			}
		}
		if !proof || !idPattern.MatchString(text(record, "last_job_id")) {
			return fail()
		}
	}
	if status == "success" && uploaded.IsZero() {
		return fail()
	}
	row.Captured = captured
	row.Uploaded = uploaded
	row.RecordScope = "Coordinator capture + upload record"
	row.Attempt = map[string]string{"": "No recorded attempt", "success": "Capture, backup + check passed", "failed": "Failed coordinator attempt", "running": "Running record; process not checked", "skipped": "Skipped; previous success preserved"}[status]
	row.Note = "Stored local verification only; no cloud check, recovery drill, process or systemd inspection was performed."
	switch {
	case !enabled:
		row.State = "inactive"
	case status == "failed":
		row.State = "failed"
	case status == "running" && at.Sub(started) > 2*time.Hour:
		row.State = "failed"
		row.Note = "Recorded run exceeds 2h; its process was not inspected."
	case status == "running":
		row.State = "running"
	case uploaded.IsZero():
		row.State = "unverified"
	case captured.After(at) || uploaded.After(at):
		row.State = "unknown"
		row.Note = "Timestamp is ahead of the read clock; freshness is uncertain."
	case at.Sub(captured) > maxAge || at.Sub(uploaded) > maxAge:
		row.State = "stale"
	default:
		row.State = "passed"
	}
	return row, nil
}
