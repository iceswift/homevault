// Package statusdata defines display data without operations or credentials.
package statusdata

import "time"

type Project struct {
	ID, Name, Kind, State                string
	Attempt, Recovery, Schedule, Note    string
	Activation, RecordScope, ProjectName string
	Captured, Uploaded                   time.Time
	Readiness                            *Readiness
	BackupCheck                          string
	HistoryKnown                         bool
	History                              []JobRecord
}

// JobRecord is bounded local evidence, never a cloud snapshot or recovery proof.
type JobRecord struct {
	ID, Status, Phase                        string
	Started, Finished                        time.Time
	BackupPassed, CheckPassed, CapturePassed bool
}

type Snapshot struct {
	ReadAt        time.Time
	Projects      []Project
	Notifications string
}

// Readiness reports bounded metadata observations, not an end-to-end success.
type Readiness struct {
	ConfigPath, Destination, Fingerprint string
	Checks                               []ReadinessCheck
}
type ReadinessCheck struct{ Name, State, Detail, Action string }
