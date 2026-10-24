# PostgreSQL + application recovery rehearsal

Experimental Linux support for one application container plus one ephemeral PostgreSQL cluster. This extends the same `rehearse` command and safety policy in RECOVERY-REHEARSAL.md. It does not use an application's production Compose file or connect to its current database.

## Plan extension

Add this optional block to a reviewed recovery plan:

```json
"postgres": {
  "image": "postgres:16-bookworm",
  "artifact": "main"
}
```

`artifact` must match a configured PostgreSQL export name in the selected HomeVault project. The restored snapshot must contain one unique preparation manifest for that project and the corresponding main.dump file. The archive is mounted read-only into the database container; the app only receives the restored-data mounts explicitly listed in the plan.

The compatible PostgreSQL 16 official image must already be installed and trusted. Other versions/custom entrypoints and extensions are not validated. Prefer pinned image digests and a server-compatible pg_dump/pg_restore version. No image pull/build is automatic.

## Fixed startup sequence

1. Restore the selected snapshot into a brand-new directory under the global operation lock.
2. Generate a random Compose project with database network_mode=none, no host ports and no production volumes. The app uses network_mode=service:database: both containers share only this isolated loopback network, not the host network or a production bridge.
3. Initialize PostgreSQL in a private 128 MiB tmpfs, using the operator's UID/GID, a read-only root filesystem, dropped capabilities and no-new-privileges. The socket directory is a separate 1 MiB tmpfs, allowing the official image's initialization flow without granting write access to its root filesystem.
4. Wait for PostgreSQL readiness, then import the trusted archive into a fixed database named recovery with pg_restore --exit-on-error --single-transaction --no-owner --no-privileges. No --create flag is used. A failed import stops before launching the app/checks and makes the report fail.
5. Start the app and execute the required health, database, media and login checks. Connect it to 127.0.0.1:5432, database recovery, user postgres. Do not reuse the production database URL or password. Authentication is trust **only inside this isolated ephemeral namespace**; this configuration must never be published or copied to production.
6. Remove this Compose project's containers/anonymous volumes and report cleanup/import/check results. The restored archive and report remain for review; PostgreSQL's tmpfs cluster is gone.

Each container is limited to 256 MiB RAM / 0.5 CPU / 64 processes. This is deliberately a small-fixture limit, not a promise that a large production database fits. A dump/archive can execute SQL chosen by its source superuser: only rehearse trusted backups/images. Network isolation and containers are not a security sandbox for malicious code.

Ownership and database privileges are intentionally not recreated. Roles/global objects, external services, production encryption keys, app migrations and application-specific login setup require a separate reviewed recovery plan. A synthetic app test does not validate Saan or another real project's recovery.

## Verification on Ubuntu (24 October 2026)

The opt-in test used official PostgreSQL 16 and a bundled synthetic Node/pg HTTP application (the already-installed DealDoor image supplies only its Node runtime and pg dependency). It did **not** run Saan or DealDoor as the PostgreSQL app.

- Created a new network-isolated source PostgreSQL fixture with a random canary and synthetic account.
- Exported using HomeVault's PostgreSQL preparation, then encrypted with Restic and stopped the source fixture.
- Restored and automatically imported into a fresh two-container recovery project.
- Checked the exact canary, database-backed health, restored media bytes through HTTP, database-backed login, issued session cookie and authenticated dashboard data. Anonymous dashboard access was denied.
- Constructed a deliberately invalid SQL archive in a separate test-only snapshot: import failed before app checks, the report remained failed, and cleanup completed.
- Asserted both network modes/no host ports/read-only DB root and verified no recovery containers remained. Source fixture containers were removed as well.

```sh
HOMEVAULT_TEST_RESTIC=/absolute/restic \
HOMEVAULT_TEST_PG_IMAGE=postgres:16-bookworm \
HOMEVAULT_TEST_PG_APP_IMAGE=your-reviewed-node-image-with-pg \
go test ./integration -run TestPostgresApplicationRehearsal -v
```

The test needs local Docker/Compose; PostgreSQL clients are supplied by the source fixture via test-only wrappers, so no host PostgreSQL packages, production sockets, ports or credentials are used. The example app image must contain /app/package.json, pg and Node.js 22-compatible fetch APIs. Test-owned temporary files are deleted by Go's test cleanup; normal CLI rehearse preserves restored files/reports.

References: [Docker Compose network modes](https://docs.docker.com/compose/how-tos/networking/), [PostgreSQL pg_restore options and trusted-input warning](https://www.postgresql.org/docs/16/app-pgrestore.html).
