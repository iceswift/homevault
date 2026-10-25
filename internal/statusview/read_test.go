package statusview

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

var testNow = time.Date(2026, 10, 25, 6, 0, 0, 0, time.UTC)

type fixture struct {
	root, plan, base, suite, engine, repo, remote string
	workflow                                      map[string]any
}

func writeDoc(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
func newFixture(t *testing.T, enabled bool) fixture {
	t.Helper()
	root := t.TempDir()
	f := fixture{root: root, plan: filepath.Join(root, "plan.json"), base: filepath.Join(root, "base.json"), suite: filepath.Join(root, "suite"), engine: filepath.Join(root, "engine"), repo: "rclone:demo:Samples", remote: filepath.Join(root, "not-opened", "rclone.conf")}
	w := map[string]any{"name": "demo", "project": "demo", "kind": "files", "enabled": enabled, "max_age_hours": 36, "files": []any{}, "trees": []any{}}
	writeDoc(t, f.base, map[string]any{"version": 1, "repository": f.repo, "rclone_config_file": f.remote, "state_dir": f.engine, "password_file": filepath.Join(root, "not-opened", "password"), "discord_webhook_file": filepath.Join(root, "not-opened", "webhook")})
	writeDoc(t, f.plan, map[string]any{"version": 1, "root": f.suite, "base_config": f.base, "homevault_binary": filepath.Join(root, "missing-homevault"), "notifications_enabled": false, "workflows": []any{w}})
	loaded, err := regularDocument(f.plan)
	if err != nil {
		t.Fatal(err)
	}
	f.workflow = loaded["workflows"].([]any)[0].(map[string]any)
	return f
}
func (f fixture) statePath() string { return filepath.Join(f.suite, "state", "demo.json") }
func (f fixture) success(t *testing.T, captured, uploaded time.Time) map[string]any {
	t.Helper()
	bound, err := binding(f.workflow, hash(f.repo+"\x00"+f.remote))
	if err != nil {
		t.Fatal(err)
	}
	attempt := map[string]any{"id": strings.Repeat("a", 24), "result": "success", "phase": "completed", "started": uploaded.Add(-time.Hour).Format(time.RFC3339Nano), "finished": uploaded.Add(time.Second).Format(time.RFC3339Nano), "captured_at": captured.Format(time.RFC3339Nano), "capture_verified": true, "upload_verified": true, "check_verified": true}
	return map[string]any{"version": 1, "binding": bound, "last_success": uploaded.Format(time.RFC3339Nano), "captured_at": captured.Format(time.RFC3339Nano), "last_job_id": strings.Repeat("b", 24), "latest": attempt, "history": []any{attempt}}
}
func first(t *testing.T, f fixture) (string, time.Time, time.Time) {
	t.Helper()
	data, err := Read(f.plan, testNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Projects) != 1 {
		t.Fatal("wrong project count")
	}
	p := data.Projects[0]
	return p.State, p.Captured, p.Uploaded
}
func TestCoordinatorFreshnessFailureAndGates(t *testing.T) {
	for _, test := range []struct {
		name     string
		captured time.Time
		mutate   func(map[string]any)
		want     string
	}{
		{"fresh", testNow.Add(-2 * time.Hour), nil, "passed"},
		{"old-source", testNow.Add(-50 * time.Hour), nil, "stale"},
		{"failed-preserves-upload", testNow.Add(-2 * time.Hour), func(r map[string]any) {
			r["latest"] = map[string]any{"id": strings.Repeat("c", 24), "result": "failed", "started": testNow.Add(-time.Minute).Format(time.RFC3339Nano), "phase": "failed"}
		}, "failed"},
		{"missing-latest", testNow.Add(-2 * time.Hour), func(r map[string]any) { r["latest"] = map[string]any{} }, "unknown"},
		{"wrong-binding", testNow.Add(-2 * time.Hour), func(r map[string]any) { r["binding"] = "wrong" }, "unknown"},
		{"verification-missing", testNow.Add(-2 * time.Hour), func(r map[string]any) { r["latest"].(map[string]any)["check_verified"] = false }, "unknown"},
		{"future", testNow.Add(time.Hour), nil, "unknown"},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newFixture(t, true)
			uploaded := testNow.Add(-time.Hour)
			record := f.success(t, test.captured, uploaded)
			if test.mutate != nil {
				test.mutate(record)
			}
			writeDoc(t, f.statePath(), record)
			state, _, got := first(t, f)
			if state != test.want {
				t.Fatalf("want %s got %s", test.want, state)
			}
			if test.want != "unknown" && !got.Equal(uploaded) {
				t.Fatal("verified upload was changed")
			}
		})
	}
	f := newFixture(t, false)
	writeDoc(t, f.statePath(), f.success(t, testNow.Add(-2*time.Hour), testNow.Add(-time.Hour)))
	state, _, _ := first(t, f)
	if state != "inactive" {
		t.Fatal("disabled workflow appeared active")
	}
}
func TestMissingRecordsDoNotCreateStateOrClaimNoBackups(t *testing.T) {
	f := newFixture(t, false)
	before, err := os.ReadFile(f.plan)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := Read(f.plan, testNow)
	if err != nil {
		t.Fatal(err)
	}
	row := snapshot.Projects[0]
	if row.State != "inactive" || row.RecordScope != "No coordinator record" || !strings.Contains(row.Note, "does not mean no cloud backup") || row.Schedule != "Unknown - systemd not inspected" || row.Recovery != "Unknown - not inspected" {
		t.Fatal("missing/unknown state overstated")
	}
	for _, path := range []string{f.suite, f.engine, filepath.Dir(f.remote)} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("reader created or opened state/secret directories")
		}
	}
	after, _ := os.ReadFile(f.plan)
	if !bytes.Equal(before, after) {
		t.Fatal("reader changed plan")
	}
}
func TestLegacyContextNeverInfersSourceFreshness(t *testing.T) {
	f := newFixture(t, true)
	finished := testNow.Add(-time.Hour)
	job := map[string]any{"id": strings.Repeat("d", 24), "started": finished.Add(-time.Minute).Format(time.RFC3339Nano), "finished": finished.Format(time.RFC3339Nano), "status": "success", "phase": "completed", "backup_passed": true, "check_passed": true}
	record := map[string]any{"version": 1, "project": "demo", "repository_id": hash(f.repo + "\x00rclone-config\x00" + f.remote), "last_success": finished.Format(time.RFC3339Nano), "latest": job, "history": []any{job}}
	path := filepath.Join(f.engine, "jobs", "demo.json")
	writeDoc(t, path, record)
	data, err := Read(f.plan, testNow)
	if err != nil {
		t.Fatal(err)
	}
	row := data.Projects[0]
	if row.State != "unverified" || row.BackupCheck != "passed" || row.SourceFreshness != "unknown" || !row.Captured.IsZero() || !row.Uploaded.Equal(finished) || row.RecordScope != "Legacy engine job only" {
		t.Fatal("legacy job treated as coordinator proof")
	}
	record["repository_id"] = "another-repository"
	writeDoc(t, path, record)
	state, _, upload := first(t, f)
	if state != "unknown" || !upload.IsZero() {
		t.Fatal("wrong repository context accepted")
	}
}
func TestRunningRecordAndMalformedInputs(t *testing.T) {
	f := newFixture(t, true)
	bound, _ := binding(f.workflow, hash(f.repo+"\x00"+f.remote))
	record := map[string]any{"version": 1, "binding": bound, "latest": map[string]any{"id": strings.Repeat("a", 24), "result": "running", "started": testNow.Add(-time.Minute).Format(time.RFC3339Nano)}, "history": []any{}}
	writeDoc(t, f.statePath(), record)
	state, _, _ := first(t, f)
	if state != "running" {
		t.Fatal("running record lost")
	}
	record["latest"].(map[string]any)["started"] = testNow.Add(-3 * time.Hour).Format(time.RFC3339Nano)
	writeDoc(t, f.statePath(), record)
	state, _, _ = first(t, f)
	if state != "failed" {
		t.Fatal("overdue running record not marked failed")
	}
	if err := os.WriteFile(f.statePath(), []byte(`{"version":1,"version":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	state, _, _ = first(t, f)
	if state != "unknown" {
		t.Fatal("duplicate keys accepted")
	}
	if err := os.WriteFile(f.statePath(), bytes.Repeat([]byte("x"), limit+1), 0600); err != nil {
		t.Fatal(err)
	}
	state, _, _ = first(t, f)
	if state != "unknown" {
		t.Fatal("oversized record accepted")
	}
}
func TestUnsafeStateSymlinkIsUnknown(t *testing.T) {
	f := newFixture(t, true)
	target := filepath.Join(f.root, "elsewhere.json")
	writeDoc(t, target, f.success(t, testNow.Add(-2*time.Hour), testNow.Add(-time.Hour)))
	if err := os.MkdirAll(filepath.Dir(f.statePath()), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, f.statePath()); err != nil {
		t.Skip("symlink not available in this environment")
	}
	state, _, _ := first(t, f)
	if state != "unknown" {
		t.Fatal("symlink was followed")
	}
}
func TestPlanErrorsAreRedactedAndTraversalRefused(t *testing.T) {
	f := newFixture(t, false)
	if err := os.WriteFile(f.plan, []byte(`{"PRIVATE_VALUE": invalid}`), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := Read(f.plan, testNow)
	if err == nil || strings.Contains(err.Error(), "PRIVATE_VALUE") || strings.Contains(err.Error(), f.root) {
		t.Fatal("raw path or JSON leaked")
	}
	f = newFixture(t, false)
	doc, _ := regularDocument(f.plan)
	doc["workflows"].([]any)[0].(map[string]any)["name"] = "../../outside"
	writeDoc(t, f.plan, doc)
	if _, err := Read(f.plan, testNow); err == nil {
		t.Fatal("traversal name accepted")
	}
}
func TestPythonBindingCompatibility(t *testing.T) {
	var vectors []struct {
		Repository string `json:"repository"`
		Config     string `json:"rclone_config_file"`
		Workflow   string `json:"workflow_json"`
		RepoID     string `json:"repo_id"`
		Binding    string `json:"binding"`
	}
	data, err := os.ReadFile("testdata/python-bindings.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	for _, v := range vectors {
		decoder := json.NewDecoder(strings.NewReader(v.Workflow))
		decoder.UseNumber()
		var workflow map[string]any
		if err = decoder.Decode(&workflow); err != nil {
			t.Fatal(err)
		}
		identity := hash(v.Repository + "\x00" + v.Config)
		if identity != v.RepoID {
			t.Fatal("repository identity differs from Python")
		}
		actual, err := binding(workflow, identity)
		if err != nil || actual != v.Binding {
			t.Fatal("workflow binding differs from Python")
		}
	}
	if _, err := pythonJSON(json.Number("36.5")); err == nil {
		t.Fatal("unsupported floating policy silently accepted")
	}
}
func TestRepeatedReadsDoNotModifyDocuments(t *testing.T) {
	f := newFixture(t, true)
	writeDoc(t, f.statePath(), f.success(t, testNow.Add(-2*time.Hour), testNow.Add(-time.Hour)))
	before := map[string][]byte{}
	modes := map[string]os.FileMode{}
	times := map[string]time.Time{}
	filepath.WalkDir(f.root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			before[path], _ = os.ReadFile(path)
			info, _ := entry.Info()
			modes[path] = info.Mode()
			times[path] = info.ModTime()
		}
		return nil
	})
	for i := 0; i < 3; i++ {
		if _, err := Read(f.plan, testNow); err != nil {
			t.Fatal(err)
		}
	}
	after := map[string][]byte{}
	filepath.WalkDir(f.root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			after[path], _ = os.ReadFile(path)
			info, _ := entry.Info()
			if info.Mode() != modes[path] || !info.ModTime().Equal(times[path]) {
				t.Fatal("mode or modification time changed")
			}
		}
		return nil
	})
	if !reflect.DeepEqual(before, after) {
		t.Fatal("read changed files")
	}
}

func TestPolicyChangeDuringReadIsNotAccepted(t *testing.T) {
	f := newFixture(t, true)
	writeDoc(t, f.statePath(), f.success(t, testNow.Add(-2*time.Hour), testNow.Add(-time.Hour)))
	changed := false
	read := func(path string) (map[string]any, error) {
		doc, err := regularDocument(path)
		if path == f.statePath() && !changed {
			plan, _ := regularDocument(f.plan)
			plan["notifications_enabled"] = true
			writeDoc(t, f.plan, plan)
			changed = true
		}
		return doc, err
	}
	if _, err := readSnapshot(f.plan, testNow, read); err == nil {
		t.Fatal("mixed policy generation was accepted")
	}
}

func TestWorkflowEvidenceDoesNotConflateGateFreshnessAndBackup(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		f := newFixture(t, enabled)
		record := f.success(t, testNow.Add(-50*time.Hour), testNow.Add(-time.Hour))
		writeDoc(t, f.statePath(), record)
		data, err := Read(f.plan, testNow)
		if err != nil {
			t.Fatal(err)
		}
		p := data.Projects[0]
		if p.BackupCheck != "passed" || p.SourceFreshness != "stale" || !strings.Contains(p.Recovery, "not inspected") {
			t.Fatal("evidence dimensions conflated")
		}
		if !enabled && p.State != "inactive" {
			t.Fatal("plan gate changed")
		}
		record["binding"] = "invalid"
		writeDoc(t, f.statePath(), record)
		data, err = Read(f.plan, testNow)
		if err != nil {
			t.Fatal(err)
		}
		if data.Projects[0].BackupCheck != "unknown" || data.Projects[0].SourceFreshness != "unknown" {
			t.Fatal("invalid binding retained evidence")
		}
	}
}
