# WhatsApp Multi-Profile Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let `nk wa` manage up to 4 independent WhatsApp accounts, selected per-command with a global `-p/--profile` flag.

**Architecture:** The `internal/whatsapp` package gains a `profile int` parameter that maps to a per-profile SQLite file (profile 1 = existing `whatsapp.db`, profiles 2-4 = `whatsapp-N.db`). The `wa` cobra command exposes a persistent `-p` flag inherited by every subcommand and validated once in a `PersistentPreRunE`; each `runWaX` threads the profile down. `status` without `-p` prints a fast local-only overview of all 4 profiles via a dedicated helper that closes its DB handle.

**Tech Stack:** Go 1.25, cobra, whatsmeow + sqlstore (modernc.org/sqlite), standard `testing`.

## Global Constraints

- Profile range is **1-4**; out-of-range is an error raised before any DB is opened or session touched. Validation is centralized in `whatsapp.ValidateProfile` and enforced for every subcommand via `waCmd.PersistentPreRunE`.
- Profile 1 MUST keep using `whatsapp.db` verbatim (zero migration of the existing session).
- Each profile is a fully isolated SQLite DB — no shared state between profiles.
- `status` overview (no `-p`) opens only already-linked local DBs and MUST NOT open network connections; it MUST close every DB handle it opens (use `whatsapp.ProfileStatus`, which `defer container.Close()`s).
- Every commit must leave `nk wa` releasable — no intermediate state where a flag is silently accepted but ignored.
- Follow the repo's existing test style: plain `testing` table-driven tests, no new test frameworks. Pure helpers are unit-tested; cobra wiring is verified by `make build` + the manual commands listed (the repo has no cobra unit tests).
- Build: `make build` produces `./nk-cli`.

### Verified facts (grounding)

- `sqlstore.Container` has `Close() error` (`container.go:239`); `*whatsmeow.Client.Disconnect()` only closes the websocket, NOT the DB — so opening a store without closing the container leaks the `sql.DB`. The overview opens up to 4 stores in one process, so it MUST close them.
- `sqlstore.New` calls `Upgrade` (a write). The overview only opens DBs that already exist and are linked (guarded by `IsLinked`), where `Upgrade` is effectively a no-op; "local-only" here means **no network**, which we preserve.
- `root.go` has no `PersistentPreRunE`, so adding one on `waCmd` is not shadowed.
- The `-p` shorthand is used by sibling commands (`add`, `share`, `rec`, `link`, shortcuts) but by **no `wa` subcommand**; cobra flags are per-command, so `wa`'s persistent `-p` does not collide.

---

### Task 1: Parametrize the `whatsapp` package by profile

Add `ValidateProfile` + the profile→filename mapping, make `GetDBPath`/`NewClient`/`IsLinked`/`DeleteDB` take a `profile int`, and extract the shared SQLite DSN. Update the existing `wa.go` call sites to pass a hardcoded `1` so behavior is unchanged and the build stays green. Flag wiring lands in Task 2.

**Files:**
- Modify: `internal/whatsapp/client.go`
- Modify: `internal/cli/wa.go` (call sites only — pass `1`)
- Test: `internal/whatsapp/client_test.go` (create)

**Interfaces:**
- Produces:
  - `func ValidateProfile(profile int) error` — nil for 1-4, else `profile must be between 1 and 4, got N`.
  - `func GetDBPath(profile int) (string, error)` — profile 1 → `<cfg>/whatsapp.db`; 2-4 → `<cfg>/whatsapp-N.db`; out-of-range → error.
  - `func NewClient(profile int, verbose bool) (*whatsmeow.Client, error)`
  - `func IsLinked(profile int) bool`
  - `func DeleteDB(profile int) error`
  - `func sqliteDSN(dbPath string) string` (unexported) — shared pragma DSN used by `NewClient` (and `ProfileStatus` in Task 3).

- [ ] **Step 1: Write the failing test**

Create `internal/whatsapp/client_test.go`:

```go
package whatsapp

import (
	"path/filepath"
	"testing"
)

func TestValidateProfile(t *testing.T) {
	for _, p := range []int{1, 2, 3, 4} {
		if err := ValidateProfile(p); err != nil {
			t.Errorf("ValidateProfile(%d) = %v, want nil", p, err)
		}
	}
	for _, p := range []int{0, 5, -1, 99} {
		if err := ValidateProfile(p); err == nil {
			t.Errorf("ValidateProfile(%d) = nil, want error", p)
		}
	}
}

func TestGetDBPathUsesProfileFilename(t *testing.T) {
	cases := []struct {
		profile  int
		wantBase string
	}{{1, "whatsapp.db"}, {2, "whatsapp-2.db"}, {3, "whatsapp-3.db"}, {4, "whatsapp-4.db"}}
	for _, c := range cases {
		path, err := GetDBPath(c.profile)
		if err != nil {
			t.Fatalf("GetDBPath(%d): %v", c.profile, err)
		}
		if base := filepath.Base(path); base != c.wantBase {
			t.Errorf("GetDBPath(%d) base = %q, want %q", c.profile, base, c.wantBase)
		}
	}
	if _, err := GetDBPath(0); err == nil {
		t.Error("GetDBPath(0): expected error")
	}
	// Isolation by construction: distinct profiles → distinct files.
	p1, _ := GetDBPath(1)
	p2, _ := GetDBPath(2)
	if p1 == p2 {
		t.Errorf("GetDBPath(1) and GetDBPath(2) must differ, both = %q", p1)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/whatsapp/ -run 'TestValidateProfile|TestGetDBPath' -v`
Expected: FAIL — `ValidateProfile` undefined and `GetDBPath` has the wrong signature (build error).

- [ ] **Step 3: Write minimal implementation**

In `internal/whatsapp/client.go`:

Add validation + the filename mapping helpers:

```go
// ValidateProfile reports whether profile is a usable WhatsApp profile (1-4).
func ValidateProfile(profile int) error {
	if profile < 1 || profile > 4 {
		return fmt.Errorf("profile must be between 1 and 4, got %d", profile)
	}
	return nil
}

// dbFileName maps a profile number to its SQLite filename. Profile 1 keeps the
// historical "whatsapp.db" name for backward compatibility; 2-4 are suffixed.
func dbFileName(profile int) (string, error) {
	if err := ValidateProfile(profile); err != nil {
		return "", err
	}
	if profile == 1 {
		return "whatsapp.db", nil
	}
	return fmt.Sprintf("whatsapp-%d.db", profile), nil
}

// sqliteDSN builds the SQLite DSN with the pragmas whatsmeow needs (WAL +
// busy_timeout so the multi-connection pool's background writers and the
// foreground readers coexist instead of returning SQLITE_BUSY).
func sqliteDSN(dbPath string) string {
	return "file:" + dbPath + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)"
}
```

Change `GetDBPath`'s signature and final return (the `configDir` switch on `runtime.GOOS` is unchanged):

```go
func GetDBPath(profile int) (string, error) {
	name, err := dbFileName(profile)
	if err != nil {
		return "", err
	}

	var configDir string
	// ... existing switch on runtime.GOOS that sets configDir (unchanged) ...

	if err := os.MkdirAll(configDir, 0700); err != nil {
		return "", err
	}

	return filepath.Join(configDir, name), nil
}
```

Update `NewClient` to take `profile` and use the shared DSN helper:

```go
func NewClient(profile int, verbose bool) (*whatsmeow.Client, error) {
	dbPath, err := GetDBPath(profile)
	if err != nil {
		return nil, fmt.Errorf("failed to get database path: %w", err)
	}

	var log waLog.Logger
	if verbose {
		log = waLog.Stdout("WhatsApp", "INFO", true)
	} else {
		log = noopLogger{}
	}

	container, err := sqlstore.New(context.Background(), "sqlite", sqliteDSN(dbPath), log)
	if err != nil {
		return nil, fmt.Errorf("failed to create session store: %w", err)
	}

	deviceStore, err := container.GetFirstDevice(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get device: %w", err)
	}

	client := whatsmeow.NewClient(deviceStore, log)
	return client, nil
}
```

Update `IsLinked` and `DeleteDB` to accept and forward `profile`:

```go
func IsLinked(profile int) bool {
	dbPath, err := GetDBPath(profile)
	if err != nil {
		return false
	}
	info, err := os.Stat(dbPath)
	if err != nil {
		return false
	}
	return info.Size() > 0
}

func DeleteDB(profile int) error {
	dbPath, err := GetDBPath(profile)
	if err != nil {
		return err
	}
	os.Remove(dbPath + "-wal")
	os.Remove(dbPath + "-shm")
	return os.Remove(dbPath)
}
```

Then in `internal/cli/wa.go`, update every call site to pass `1` (no behavior change yet):
- `runWaLink`: `whatsapp.NewClient(1, false)`
- `runWaSend`: `whatsapp.NewClient(1, false)`
- `runWaLs`: `whatsapp.NewClient(1, false)`
- `runWaUnlink`: `whatsapp.IsLinked(1)`, `whatsapp.NewClient(1, false)`, `whatsapp.DeleteDB(1)`
- `runWaStatus`: `whatsapp.IsLinked(1)`, `whatsapp.NewClient(1, false)`

- [ ] **Step 4: Run tests + build to verify they pass**

Run: `go test ./internal/whatsapp/ -v && make build`
Expected: PASS, and `Built: nk-cli`.

- [ ] **Step 5: Commit**

```bash
git add internal/whatsapp/client.go internal/whatsapp/client_test.go internal/cli/wa.go
git commit -m "feat(wa): parametrize whatsapp session store by profile (1-4)"
```

---

### Task 2: Add the `-p/--profile` flag, validate it, and thread it through every command

Add a persistent `-p` flag on `waCmd`, validate it once in `PersistentPreRunE` (so `link/send/ls/status/unlink -p 5` all get a clean range error), and pass the profile to the `whatsapp` package in every runner — including `status`'s detail path. "Not linked" errors name the profile. After this task, `nk wa <cmd> -p N` works for all commands; `status` with no flag keeps its current profile-1 detail output (the multi-profile overview is added in Task 3). This keeps every commit releasable.

**Files:**
- Modify: `internal/cli/wa.go`
- Test: `internal/cli/wa_test.go` (create — covers the pure error-message helper)

**Interfaces:**
- Consumes: `whatsapp.ValidateProfile`, `whatsapp.NewClient(profile, verbose)`, `whatsapp.IsLinked(profile)`, `whatsapp.DeleteDB(profile)` from Task 1.
- Produces:
  - Persistent int flag `"profile"` (shorthand `p`, default `1`) on `waCmd`, validated in `waCmd.PersistentPreRunE`.
  - `func notLinkedError(profile int) error` — `profile 1` → `WhatsApp not linked. Run "nk wa link" first`; `profile N>1` → `WhatsApp profile N not linked. Run "nk wa link -p N" first`.

- [ ] **Step 1: Write the failing test**

Create `internal/cli/wa_test.go`:

```go
package cli

import (
	"strings"
	"testing"
)

func TestNotLinkedError(t *testing.T) {
	if got := notLinkedError(1).Error(); got != `WhatsApp not linked. Run "nk wa link" first` {
		t.Errorf("profile 1 message = %q", got)
	}
	got := notLinkedError(2).Error()
	if !strings.Contains(got, "profile 2") || !strings.Contains(got, `nk wa link -p 2`) {
		t.Errorf("profile 2 message = %q, want it to mention profile 2 and -p 2", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run TestNotLinkedError -v`
Expected: FAIL — `notLinkedError` undefined (build error).

- [ ] **Step 3: Write minimal implementation**

In `internal/cli/wa.go`:

Add the helper near the top of the file (after the `var (...)` block):

```go
// notLinkedError builds the "not linked" error, naming the profile when it is
// not the default so the user runs "nk wa link" against the right account.
func notLinkedError(profile int) error {
	if profile == 1 {
		return fmt.Errorf(`WhatsApp not linked. Run "nk wa link" first`)
	}
	return fmt.Errorf(`WhatsApp profile %d not linked. Run "nk wa link -p %d" first`, profile, profile)
}
```

Register the persistent flag and validation at the end of `addWaCommands`, before `rootCmd.AddCommand(waCmd)`:

```go
	waCmd.PersistentFlags().IntP("profile", "p", 1, "WhatsApp profile to use (1-4)")
	// Validate the profile once for every wa subcommand, before any DB/session
	// work. root.go has no PersistentPreRunE, so this is not shadowed.
	waCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		profile, _ := cmd.Flags().GetInt("profile")
		return whatsapp.ValidateProfile(profile)
	}
```

Thread the profile through each runner. As the first line of `runWaLink`, `runWaSend`, `runWaLs`, `runWaUnlink`, and `runWaStatus`:

```go
	profile, _ := cmd.Flags().GetInt("profile")
```

Then replace the hardcoded `1` from Task 1 with `profile`:
- `runWaLink`: `whatsapp.NewClient(profile, false)`
- `runWaSend`: `whatsapp.NewClient(profile, false)`; change the not-linked branch to `return notLinkedError(profile)`
- `runWaLs`: `whatsapp.NewClient(profile, false)`; change the not-linked branch to `return notLinkedError(profile)`
- `runWaUnlink`: `whatsapp.IsLinked(profile)`, `whatsapp.NewClient(profile, false)`, `whatsapp.DeleteDB(profile)`
- `runWaStatus`: `whatsapp.IsLinked(profile)`, `whatsapp.NewClient(profile, false)`. Also update its printed lines to name the profile, e.g. replace `fmt.Println("WhatsApp: Linked")` with `fmt.Printf("WhatsApp profile %d: Linked\n", profile)` and the two "Not linked" branches with `fmt.Printf("WhatsApp profile %d: Not linked\n", profile)` followed by `fmt.Printf("Run \"nk wa link -p %d\" to connect this account.\n", profile)`. (The no-flag overview replaces the top of this function in Task 3; the detail body stays.)

Update the help text. Append to `waCmd.Long` examples:

```
  nk wa link -p 2                     Link a second account (profile 2)
  nk wa send -p 2 7778887788 "Hi"     Send from profile 2
  nk wa status                        Show status of all profiles
```

Append to `sendCmd.Long` examples:

```
  nk wa send -p 2 14255687870 "Hi"              # send from profile 2
```

- [ ] **Step 4: Run tests + build**

Run: `go test ./internal/cli/ -run TestNotLinkedError -v && make build`
Expected: PASS, `Built: nk-cli`.

- [ ] **Step 5: Manual smoke check**

Run: `./nk-cli wa --help` and `./nk-cli wa send --help`
Expected: help shows the `-p, --profile` flag and the new examples.
Run: `./nk-cli wa status -p 5`
Expected: error `profile must be between 1 and 4, got 5` (PersistentPreRunE rejects it before any DB work).
Run: `./nk-cli wa status -p 1`
Expected: detailed status for profile 1 (your existing session), with the "Verifying connection..." spinner.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/wa.go internal/cli/wa_test.go
git commit -m "feat(wa): add validated -p/--profile flag, thread through all commands"
```

---

### Task 3: Multi-profile `status` overview (no `-p`)

When the user runs `status` without `-p`, print a fast, local-only overview of all 4 profiles — opening only already-linked DBs and closing each handle. With `-p N`, keep the detail behavior wired in Task 2.

**Files:**
- Modify: `internal/whatsapp/client.go` (add `ProfileStatus`)
- Modify: `internal/cli/wa.go` (`runWaStatus` overview branch + `formatProfileLine`)
- Test: `internal/cli/wa_test.go` (add `formatProfileLine` test); `internal/whatsapp/client_test.go` (add `ProfileStatus` unlinked-profile test)

**Interfaces:**
- Consumes: the `"profile"` persistent flag from Task 2; `whatsapp.IsLinked`, `whatsapp.sqliteDSN`, `whatsapp.GetDBPath` from Task 1.
- Produces:
  - `func (whatsapp) ProfileStatus(profile int) (linked bool, jid string, err error)` — opens the profile's store **only if already linked**, reads the device JID, and `defer container.Close()`s. Never connects to the network. Returns `(false, "", nil)` for an unlinked profile; an error only for an out-of-range profile or a store-open failure.
  - `func formatProfileLine(profile int, linked bool, id string) string` — one overview row: `"  2  Linked      (<jid>)"` or `"  3  Not linked"`.

- [ ] **Step 1: Write the failing tests**

Add to `internal/whatsapp/client_test.go`:

```go
func TestProfileStatusUnlinked(t *testing.T) {
	// A profile with no DB file is reported unlinked, with no error.
	// Profile 4 is not linked in any dev/test environment.
	linked, jid, err := ProfileStatus(4)
	if err != nil {
		t.Fatalf("ProfileStatus(4): unexpected error %v", err)
	}
	if linked || jid != "" {
		t.Errorf("ProfileStatus(4) = (%v, %q), want (false, \"\")", linked, jid)
	}
	if _, _, err := ProfileStatus(0); err == nil {
		t.Error("ProfileStatus(0): expected range error")
	}
}
```

Add to `internal/cli/wa_test.go`:

```go
func TestFormatProfileLine(t *testing.T) {
	linked := formatProfileLine(2, true, "5217...@s.whatsapp.net")
	if !strings.Contains(linked, "2") || !strings.Contains(linked, "Linked") ||
		!strings.Contains(linked, "5217...@s.whatsapp.net") {
		t.Errorf("linked line = %q", linked)
	}
	notLinked := formatProfileLine(3, false, "")
	if !strings.Contains(notLinked, "3") || !strings.Contains(notLinked, "Not linked") {
		t.Errorf("not-linked line = %q", notLinked)
	}
	if strings.Contains(notLinked, "(") {
		t.Errorf("not-linked line should not show an id: %q", notLinked)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/whatsapp/ -run TestProfileStatus -v && go test ./internal/cli/ -run TestFormatProfileLine -v`
Expected: FAIL — `ProfileStatus` and `formatProfileLine` undefined.

- [ ] **Step 3: Write minimal implementation**

In `internal/whatsapp/client.go`, add (it reuses `sqliteDSN`, `noopLogger`, `context`, `sqlstore` — all already in the file):

```go
// ProfileStatus reports a profile's link state and device JID by reading only
// the local SQLite store — it never connects to WhatsApp. It opens the store
// only when the profile is already linked and always closes the DB handle.
func ProfileStatus(profile int) (linked bool, jid string, err error) {
	if err := ValidateProfile(profile); err != nil {
		return false, "", err
	}
	if !IsLinked(profile) {
		return false, "", nil
	}
	dbPath, err := GetDBPath(profile)
	if err != nil {
		return false, "", err
	}
	container, err := sqlstore.New(context.Background(), "sqlite", sqliteDSN(dbPath), noopLogger{})
	if err != nil {
		return false, "", err
	}
	defer container.Close()

	device, err := container.GetFirstDevice(context.Background())
	if err != nil || device.ID == nil {
		return false, "", nil
	}
	return true, device.ID.String(), nil
}
```

In `internal/cli/wa.go`, add the formatter:

```go
// formatProfileLine renders one row of the multi-profile status overview.
func formatProfileLine(profile int, linked bool, id string) string {
	if !linked {
		return fmt.Sprintf("  %d  Not linked", profile)
	}
	return fmt.Sprintf("  %d  Linked      (%s)", profile, id)
}
```

At the top of `runWaStatus` (after reading `profile`), add the overview branch before the existing detail body from Task 2:

```go
	profile, _ := cmd.Flags().GetInt("profile")

	// No explicit -p: fast, local-only overview of all profiles (no network).
	if !cmd.Flags().Changed("profile") {
		fmt.Println("WhatsApp profiles:")
		for p := 1; p <= 4; p++ {
			linked, id, _ := whatsapp.ProfileStatus(p)
			fmt.Println(formatProfileLine(p, linked, id))
		}
		return nil
	}

	// Explicit -p N: detailed status for that profile (verifies live connection).
	// ... existing detail body from Task 2, unchanged ...
```

- [ ] **Step 4: Run tests + build**

Run: `go test ./internal/... && make build`
Expected: PASS, `Built: nk-cli`.

- [ ] **Step 5: Manual smoke check**

Run: `./nk-cli wa status`
Expected: a 4-row overview; profile 1 shows `Linked (…)` (your existing session), profiles 2-4 show `Not linked`. No spinner, no network.
Run: `./nk-cli wa status -p 1`
Expected: detailed status for profile 1 with the "Verifying connection..." spinner (detail path from Task 2).

- [ ] **Step 6: Commit**

```bash
git add internal/whatsapp/client.go internal/whatsapp/client_test.go internal/cli/wa.go internal/cli/wa_test.go
git commit -m "feat(wa): multi-profile status overview, closing each DB handle"
```

---

## Self-Review

**Spec coverage:**
- Flag `-p/--profile` 1-4 → Task 2 (flag), Task 1 (`ValidateProfile`). ✓
- Range error before any DB/session → Task 1 `ValidateProfile` + Task 2 `PersistentPreRunE` (covers status & unlink too, not just path-opening commands). ✓
- Profile 1 = `whatsapp.db`, 2-4 = `whatsapp-N.db` → Task 1 `dbFileName`. ✓
- Isolated DBs → Task 1 (distinct file per profile, asserted in test). ✓
- Per-profile error messages → Task 2 `notLinkedError` + status lines. ✓
- Help examples with `-p` → Task 2. ✓
- `status` overview (local-only, all 4, no network, closes handles) + `-p` detail → Task 3 (`ProfileStatus`) / Task 2 (detail). ✓
- Backward compat: existing session untouched (profile 1 path unchanged; callers default to 1). ✓

**Releasability:** Task 1 keeps behavior identical (hardcoded 1). After Task 2, every command honors `-p` and `status` with no flag keeps current profile-1 detail (correct, not misleading). Task 3 only adds the no-flag overview branch. No commit accepts a flag it ignores. ✓

**Resource safety:** The only path opening multiple stores in one process is the overview, which uses `ProfileStatus` with `defer container.Close()`. One-shot commands (`link/send/ls/status -p N`) open one store and exit, matching existing behavior. ✓

**Placeholder scan:** none — all steps carry concrete code/commands.

**Type consistency:** `profile int` first arg across `ValidateProfile`/`GetDBPath`/`NewClient`/`IsLinked`/`DeleteDB`/`ProfileStatus`; flag name `"profile"` read consistently; `formatProfileLine(int, bool, string) string` and `ProfileStatus(int) (bool, string, error)` match their call sites in `runWaStatus`.
