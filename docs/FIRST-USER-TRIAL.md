# First-user trial

Help us find steps that are confusing before trusting HomeVault with real data.
This trial uses generated files and a new local encrypted repository. No Google
account, Docker, server credentials or existing project configuration is needed.

**Current evidence:** automated Linux amd64 package/restore rehearsals exist.
An independent first-user trial is still pending. Maintainer and CI runs do not
count as independent usability feedback. This guide is on main and will be bundled
in future packages; it is not inside the existing `v0.1.0-preview.1` archive.

## Choose one starting point

| Track | Use it for | Requirements |
| --- | --- | --- |
| Published Preview | First installation and byte-verified local recovery | Linux amd64, Bash, Python 3.11+, Restic; no Go compiler |
| Current-main/CI build | Feedback on redesigned TUI, history and readiness | Same runtime dependencies; source builds also need Git and Go 1.24.5+ |

Use [release verification](RELEASING.md) to choose/check a prebuilt archive, or
the [source Quick Start](QUICKSTART.md) to build. Do not mix binaries from the two
tracks. Record `homevault version` and the archive/tag or full source commit.
For a CI package, its `RELEASE.json` identifies the commit. CI artifacts are
experimental and their downloads may require GitHub sign-in. The published Preview
does not contain `tui-jobs`, `tui-status` or `tui-demo`; that is not an install bug.
ARM64 packages are cross-built but native runtime testing remains unverified.

## Before starting

Use a spare Linux machine/VM or a terminal session with ordinary user privileges.
Dependency installation is a separate step and may need administrator access;
the trial itself does not. Record OS/architecture, relevant tool versions and how
much time dependency setup takes. Do not use production data/config or perform
cloud onboarding during this trial.

Start a second timer when the prerequisites are available. Note the first step
you cannot complete unaided. Asking for help is useful feedback: record what was
unclear and whether the documentation or another person resolved it.

## 1. Complete the local walkthrough

For a verified extracted package, run from its directory:

```sh
./homevault version
HOMEVAULT_QUICKSTART_BINARY="$PWD/homevault" bash scripts/quickstart-smoke.sh
```

For a source checkout after building the executable:

```sh
./bin/homevault version
bash scripts/quickstart-smoke.sh
```

The script creates a **new random sandbox**, prints where it lives, backs up its
generated files, checks repository metadata, restores into a separate destination
and compares bytes. It keeps the sandbox for inspection. Each run creates a new
sandbox; do not adapt it to an existing repository to make a failed step pass.

The required final result is:

```text
PASS: restored proof.txt matches the generated source byte for byte
```

A screenshot of a green status or a successful upload does not replace that
comparison. If the command fails or that line is missing, report the last completed
step and a short redacted error. Keep the sandbox; no cleanup or retry is required
to submit useful feedback. This proves sample-file recovery only, not full-server
or application recovery.

## 2. Try the redesigned TUI (current main/CI only)

Use the sandbox directory printed above. Replace the example config path below
with its generated `lab/homevault.json`. For a source build substitute
`./bin/homevault` for `./homevault`.

```sh
./homevault --config /printed/sandbox/lab/homevault.json tui-jobs --context lab
```

Try these tasks without running another backup:

1. Explain the three fields: backup/check, source freshness and application recovery.
   For this file-only demo a recorded backup/check can pass while freshness is
   Unknown and recovery is Not inspected by the screen.
2. Press `h`, find the recorded attempt and its separate backup/check results,
   then Esc to return.
3. Press `d`, identify the selected config/destination and one property that
   readiness explicitly did **not** test. Scroll with Page Down, then Esc.
4. Press `b` to open review, then Esc without typing the project ID. Opening or
   cancelling review must not start a job. An unexpected start is a bug; stop here.
5. Try a narrow terminal (about 60 columns), filtering with `/`, and returning
   to the same project. Use `q` to exit.

`LAB (operator label)` is an explicit display label, not a protection against
choosing the wrong configuration. Use only the generated sandbox during the trial.
The real-plan viewer `tui-status` is a separate read-only mode and is not required
here; do not borrow somebody else's server plan just to finish this test.

## 3. Submit feedback

Open the repository's [First-user trial feedback](https://github.com/iceswift/homevault/issues/new?template=first_user_trial.yml) issue form. Do not submit an
issue automatically from a script. The form distinguishes independent participants,
maintainers/operators and automated runs. Report:

- Version/commit, installation track, OS/architecture and terminal size.
- Dependency setup time separately from time to the verified restore.
- Whether the exact byte-comparison PASS line appeared, failed, or was not reached.
- Where you needed help, with the step and the instruction you expected instead.
- Which optional TUI tasks you completed, skipped or found confusing.

Use generated examples and short redacted errors. Do not attach credentials,
private configurations, real backups or full logs. See [security reporting](../SECURITY.md)
for security-sensitive findings.

## Maintainer acceptance

An independent trial is complete only when a person outside the implementation
work records the tested version, follows the walkthrough, reports the byte-verified
restore result and describes friction/help. TUI feedback can remain separately
pending if the participant used the published Preview. A failed or abandoned trial
is valuable evidence, but does not count as successful onboarding.

Track findings as issues with an affected step, proposed correction and a retest
result. Keep automated rehearsal, maintainer validation and independent feedback
separate in [testing evidence](TESTING.md) and the [roadmap](ROADMAP.md). Do not claim
the product is easy for new users merely because CI passed.
