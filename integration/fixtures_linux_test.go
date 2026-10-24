//go:build linux

package integration

import _ "embed"

//go:embed fixtures/recovery-app.py
var fixtureApp []byte

//go:embed fixtures/postgres-app.cjs
var postgresFixtureApp []byte

//go:embed fixtures/disaster-drill.py
var disasterSupervisor []byte
