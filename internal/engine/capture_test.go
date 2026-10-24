package engine

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testCapturePlan(t *testing.T) CapturePlan {
	t.Helper()
	return CapturePlan{ContainerID: strings.Repeat("a", 64), ImageID: "sha256:" + strings.Repeat("b", 64), DataRoot: filepath.Join(t.TempDir(), "data"), WindowStart: "23:50", WindowMinutes: 20, Timezone: "Asia/Bangkok", MaxPauseSeconds: 30}
}
func TestCaptureWindowAndIdentityValidation(t *testing.T) {
	p := testCapturePlan(t)
	if err := p.validate(); err != nil {
		t.Fatal(err)
	}
	loc, _ := time.LoadLocation("Asia/Bangkok")
	for _, v := range []struct {
		hour, minute int
		allowed      bool
	}{{23, 49, false}, {23, 50, true}, {0, 9, true}, {0, 10, false}, {12, 0, false}} {
		if p.inWindow(time.Date(2026, 10, 24, v.hour, v.minute, 0, 0, loc)) != v.allowed {
			t.Fatal("window mismatch", v)
		}
	}
	p.MaxPauseSeconds = 61
	if p.validate() == nil {
		t.Fatal("unsafe pause bound accepted")
	}
	p.MaxPauseSeconds = 30
	p.ContainerID = "production-app"
	if p.validate() == nil {
		t.Fatal("mutable container name accepted")
	}
}
