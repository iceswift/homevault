package jobops

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"homevault/internal/statusdata"
)

func checkNamed(t *testing.T, r *statusdata.Readiness, name string) statusdata.ReadinessCheck {
	t.Helper()
	for _, c := range r.Checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("missing readiness item: %s", name)
	return statusdata.ReadinessCheck{}
}

func TestMissingToolsKeepDiagnosticsOpenButBlockJobs(t *testing.T) {
	b, _ := backendFixture(t)
	b.cfg.ResticBinary = filepath.Join(filepath.Dir(b.path), "missing-restic")
	data, _ := json.Marshal(b.cfg)
	os.WriteFile(b.path, data, 0600)
	b, err := New(b.path)
	if err != nil || b.engine != nil {
		t.Fatal("missing tool prevented diagnostics or left usable engine")
	}
	view, err := b.Load()
	if err != nil {
		t.Fatal(err)
	}
	p := view.Projects[0]
	if checkNamed(t, p.Readiness, "Restic").State != "ATTENTION" || checkNamed(t, p.Readiness, "Job engine").State != "BLOCKED" || p.BackupCheck != "unknown" {
		t.Fatal("missing tool incorrectly shown ready")
	}
	review, err := b.Review("demo")
	if err != nil || review.Allowed || review.Token != "" {
		t.Fatal("missing tools allowed review")
	}
	if _, err = b.Start(review.Token); err == nil {
		t.Fatal("missing tools allowed job")
	}
	if _, err = os.Stat(b.cfg.StateDir); !os.IsNotExist(err) {
		t.Fatal("diagnostics wrote state")
	}
}

func TestReadinessMetadataIsBoundedAndDoesNotExposeCredentials(t *testing.T) {
	b, f := backendFixture(t)
	for i := 0; i < 30; i++ {
		b.cfg.Projects[0].Paths = append(b.cfg.Projects[0].Paths, filepath.Join(filepath.Dir(b.path), "missing-source"))
	}
	data, err := b.Load()
	if err != nil {
		t.Fatal(err)
	}
	r := data.Projects[0].Readiness
	if len(r.Checks) > 32 || checkNamed(t, r, "Additional sources").State != "NOT CHECKED" || checkNamed(t, r, "End-to-end readiness").State != "NOT CHECKED" {
		t.Fatal("readiness unbounded or overstated")
	}
	encoded, _ := json.Marshal(r)
	if strings.Contains(string(encoded), "generated-test-key") || f.calls != 0 {
		t.Fatal("secret exposed or job invoked")
	}
	if _, err = os.Stat(b.cfg.StateDir); !os.IsNotExist(err) {
		t.Fatal("metadata check created state")
	}
	if _, err = os.Stat(b.cfg.Repository); !os.IsNotExist(err) {
		t.Fatal("metadata check initialized repository")
	}
}
