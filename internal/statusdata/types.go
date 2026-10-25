// Package statusdata defines display data without operations or credentials.
package statusdata

import "time"

type Project struct {
	ID, Name, Kind, State                string
	Attempt, Recovery, Schedule, Note    string
	Activation, RecordScope, ProjectName string
	Captured, Uploaded                   time.Time
}

type Snapshot struct {
	ReadAt        time.Time
	Projects      []Project
	Notifications string
}
