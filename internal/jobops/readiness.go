package jobops

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"homevault/internal/config"
	"homevault/internal/statusdata"
)

// readiness never executes tools, opens credential contents, traverses source
// trees, creates directories or probes a repository. Permission observations are
// metadata only; ACLs, effective access and writable space are explicitly unknown.
func (b *Backend) readiness(p config.Project) *statusdata.Readiness {
	r := &statusdata.Readiness{ConfigPath: b.path, Destination: b.cfg.Repository, Fingerprint: b.digest[:12]}
	add := func(name, state, detail, action string) {
		r.Checks = append(r.Checks, statusdata.ReadinessCheck{Name: name, State: state, Detail: detail, Action: action})
	}
	add("Configuration", "OBSERVED", "Loaded and validated; fingerprint matches this session.", "Restart after editing config; review the destination before confirming a job.")
	tool := func(label, name, fallback string) {
		if name == "" {
			name = fallback
		}
		if _, err := exec.LookPath(name); err != nil {
			add(label, "ATTENTION", "Executable not found by local lookup; no program was run.", "Install the tool or correct its configured binary path, then restart this view.")
		} else {
			add(label, "OBSERVED", "Executable found by local lookup; version and execution not tested.", "Use a separately reviewed CLI check if runtime verification is needed.")
		}
	}
	tool("Restic", b.cfg.ResticBinary, "restic")
	if strings.HasPrefix(b.cfg.Repository, "rclone:") {
		tool("rclone", b.cfg.RcloneBinary, "rclone")
		if b.cfg.RcloneConfigFile == "" {
			add("rclone credentials", "NOT CHECKED", "No explicit credential file; default/environment credentials were not inspected.", "Review rclone credential selection separately; do not paste tokens into chat.")
		} else {
			r.Checks = append(r.Checks, credentialMetadata("rclone credentials", b.cfg.RcloneConfigFile))
		}
	}
	r.Checks = append(r.Checks, credentialMetadata("Repository key", b.cfg.PasswordFile))
	if b.cfg.DiscordWebhookFile != "" {
		r.Checks = append(r.Checks, credentialMetadata("Notification file", b.cfg.DiscordWebhookFile))
	}
	if b.engine == nil {
		add("Job engine", "BLOCKED", "Tools or configuration could not be prepared. No jobs can start in this session.", "Fix local tool/config issues and restart; readiness does not repair files.")
	} else {
		add("Job engine", "OBSERVED", "Constructed locally; no tool executed and no repository contacted.", "A confirmed job still performs its own validation and locking.")
	}
	if reason := b.blocked(p); reason != "" {
		add("Manual job scope", "BLOCKED", reason, "Use the reviewed capture/database/recovery workflow separately.")
	} else {
		add("Manual job scope", "OBSERVED", "Configured as file-only. Live database content is not detected by this check.", "Confirm source directories exclude live database storage before starting.")
	}
	for i, source := range p.Paths {
		if i >= 16 {
			add("Additional sources", "NOT CHECKED", "Only the first 16 configured source paths are inspected in this bounded view.", "Review the full source list in the selected configuration.")
			break
		}
		info, err := os.Lstat(source)
		switch {
		case err != nil:
			add("Source: "+source, "ATTENTION", "Directory metadata unavailable; missing path or permission issue.", "Verify this path and the invoking user's access; this view changes no permissions.")
		case info.Mode()&os.ModeSymlink != 0:
			add("Source: "+source, "NOT CHECKED", "Symlink found; target contents and access were not inspected.", "Review the target and permissions separately.")
		case !info.IsDir():
			add("Source: "+source, "ATTENTION", "Source is not a directory.", "Correct the source path; the manual engine expects directories.")
		default:
			add("Source: "+source, "OBSERVED", "Directory metadata found; contents, consistency and effective read access not checked.", "Review read access and app consistency separately.")
		}
	}
	add("State directory", "NOT CHECKED", directoryObservation(b.cfg.StateDir), "Verify write access for the invoking user; no write probe or directory creation was performed.")
	if !strings.HasPrefix(b.cfg.Repository, "rclone:") {
		add("Local destination", "NOT CHECKED", directoryObservation(b.cfg.Repository), "Review the destination and initialization separately; this view never initializes a repository.")
	}
	add("End-to-end readiness", "NOT CHECKED", "Effective permissions/ACLs, free space, locks, key correctness, repository initialization, cloud access and restore are untested.", "Treat these observations as diagnostics, not permission to skip review or proof a backup will succeed.")
	return r
}

func credentialMetadata(label, path string) statusdata.ReadinessCheck {
	c := statusdata.ReadinessCheck{Name: label, State: "ATTENTION", Detail: "Credential metadata unavailable, empty or not a regular file; contents were not opened.", Action: "Review this credential file locally. Do not display its contents or paste secrets into chat."}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return c
	}
	if runtime.GOOS == "windows" {
		c.State, c.Detail = "NOT CHECKED", "Regular non-empty file found; Windows ACL privacy and effective access not evaluated. Contents were not opened."
	} else if info.Mode().Perm()&0077 != 0 {
		c.Detail = "Group/other permission bits are present; contents were not opened."
		c.Action = "Review ownership and restrict access locally where appropriate; no chmod is performed here."
	} else {
		c.State, c.Detail = "OBSERVED", "Regular non-empty file with private mode bits; contents, ownership and effective access not evaluated."
	}
	return c
}

func directoryObservation(path string) string {
	info, err := os.Lstat(filepath.Clean(path))
	if os.IsNotExist(err) {
		return "Directory is absent; no directory was created. Parent write access is not evaluated."
	}
	if err != nil {
		return "Directory metadata unavailable; permission or path issue. No write access was tested."
	}
	if !info.IsDir() {
		return "Path is not a plain directory (possibly a symlink); target and write access not inspected."
	}
	return "Directory metadata found; effective write access, free space and initialization not inspected."
}
