// Package statusdata defines display data without operations or credentials.
package statusdata

import "time"

type Project struct {
	ID, Name, Kind, State                string
	Attempt, Recovery, Schedule, Note    string
	Activation, RecordScope, ProjectName string
	Captured, Uploaded                   time.Time
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
