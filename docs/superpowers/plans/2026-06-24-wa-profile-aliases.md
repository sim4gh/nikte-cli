# WhatsApp Profile Aliases Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let `nk wa -p` accept a memorable alias (e.g. `trabajo`) as well as a profile number 1-4, managed with a new `nk wa alias` command.

**Architecture:** A new `internal/whatsapp/aliases.go` persists a profile→alias map in `wa_aliases.json` (atomic writes) and resolves a raw `-p` value (number or alias) to a profile int. The `-p` cobra flag changes from `IntP` to `StringP`; a single `profileFromCmd` helper replaces every direct flag read. A new `nk wa alias` subcommand creates/lists/clears aliases, and profile-naming output switches to a `profileLabel` helper that shows the alias when one exists.

**Tech Stack:** Go 1.25, cobra, standard `testing`, encoding/json.

## Global Constraints

- `-p` accepts a number (1-4) **or** an alias; the number always works as a fallback. Validation/resolution happens in `waCmd.PersistentPreRunE` before any DB/session work.
- Alias format: must match `^[A-Za-z0-9_-]{1,32}$`, must NOT be purely numeric (`^\d+$`), must NOT be the reserved word `all` (case-insensitive). `ValidateAlias` does NOT check uniqueness.
- Alias assignment is **move-semantics**: assigning a name removes it (case-insensitive) from any other profile. Uniqueness is a postcondition of `SetAlias`, not a validation error.
- Alias resolution is **deterministic**: iterate profiles 1→4, first case-insensitive match wins.
- `wa_aliases.json`: absent → empty map (`nil` error); present-but-corrupt → error. **Reads** tolerate the error (empty map); the **`nk wa alias` write command aborts** on it (never overwrites a corrupt file). Writes are **atomic** (temp file + `os.Rename`, mode 0600).
- A name that trims to empty means **remove the alias** (bypasses `ValidateAlias`).
- Aliasing does NOT require a linked DB — any profile 1-4 can be aliased before linking.
- Profile 1 keeps using `whatsapp.db`; existing numeric `-p N` behavior is unchanged.
- Build: `make build` produces `./nk-cli`. Tests: `go test ./internal/...`.
- Follow the repo's existing test style: plain `testing` table-driven tests, no new frameworks.

### Verified facts (grounding)

- `internal/cli/wa.go` reads the flag in 5 places: `addWaCommands` PersistentPreRunE (line ~136), `runWaLink` (~145), `runWaSend` (~234), `runWaLs` (~532), `runWaUnlink` (~732), `runWaStatus` (~773). All currently use `cmd.Flags().GetInt("profile")` with the error ignored. Flag registered at `wa.go:132` as `IntP("profile", "p", 1, ...)`.
- `runWaStatus` routes overview-vs-detail with `cmd.Flags().Changed("profile")` (wa.go:776) — this is type-independent, so it survives the `StringP` change.
- The config dir resolution lives inline inside `GetDBPath` (`internal/whatsapp/client.go:77-108`): darwin → `~/Library/Application Support/nikte`, windows → `%APPDATA%/nikte`, else → `$XDG_CONFIG_HOME/nikte` (or `~/.config/nikte`).
- Existing helpers: `whatsapp.ValidateProfile(int) error`, `whatsapp.GetDBPath(int)`, `whatsapp.IsLinked(int)`, `whatsapp.ProfileStatus(int)`; CLI `notLinkedError(int) error` (wa.go:37), `formatProfileLine(int, bool, string) string` (wa.go:765).

---

### Task 1: Alias storage, validation, and profile resolution (`whatsapp` package)

Extract the config-dir logic from `GetDBPath` into a shared helper, then add `aliases.go` with the alias map type, format validation, deterministic resolution, atomic save, and `ResolveProfile`. Pure package logic with hermetic tests (temp dirs) — no CLI change, build stays green.

**Files:**
- Modify: `internal/whatsapp/client.go` (extract `configDir()` from `GetDBPath`)
- Create: `internal/whatsapp/aliases.go`
- Test: `internal/whatsapp/aliases_test.go`

**Interfaces:**
- Produces:
  - `func configDir() (string, error)` (unexported) — the per-OS nikte config dir, `MkdirAll`-ed. `GetDBPath` now calls it.
  - `type Aliases map[int]string`
  - `func ValidateAlias(name string) error`
  - `func (a Aliases) AliasOf(profile int) string`
  - `func (a Aliases) ResolveAlias(name string) (int, bool)`
  - `func (a Aliases) SetAlias(profile int, name string) Aliases`
  - `func (a Aliases) ClearAlias(profile int) Aliases`
  - `func LoadAliases() (Aliases, error)` / `func (a Aliases) Save() error`
  - `func ResolveProfile(raw string) (int, error)`

- [ ] **Step 1: Write the failing tests**

Create `internal/whatsapp/aliases_test.go`:

```go
package whatsapp

import (
	"path/filepath"
	"testing"
)

func TestValidateAlias(t *testing.T) {
	good := []string{"trabajo", "work-phone", "cuenta_2", "a", "A1b2_-"}
	for _, n := range good {
		if err := ValidateAlias(n); err != nil {
			t.Errorf("ValidateAlias(%q) = %v, want nil", n, err)
		}
	}
	bad := []string{"", "  ", "2", "42", "all", "ALL", "con espacio", "emoji😀", "has.dot",
		"this-name-is-way-too-long-to-be-valid-x"}
	for _, n := range bad {
		if err := ValidateAlias(n); err == nil {
			t.Errorf("ValidateAlias(%q) = nil, want error", n)
		}
	}
}

func TestResolveAndSetAlias(t *testing.T) {
	a := Aliases{}.SetAlias(2, "trabajo").SetAlias(3, "personal")
	if a.AliasOf(2) != "trabajo" {
		t.Errorf("AliasOf(2) = %q", a.AliasOf(2))
	}
	// case-insensitive resolution
	if p, ok := a.ResolveAlias("TRABAJO"); !ok || p != 2 {
		t.Errorf("ResolveAlias(TRABAJO) = (%d,%v), want (2,true)", p, ok)
	}
	if _, ok := a.ResolveAlias("nope"); ok {
		t.Error("ResolveAlias(nope) should be false")
	}
	// move-semantics: assigning trabajo to 4 removes it from 2
	a = a.SetAlias(4, "trabajo")
	if a.AliasOf(2) != "" {
		t.Errorf("after move, AliasOf(2) = %q, want empty", a.AliasOf(2))
	}
	if p, _ := a.ResolveAlias("trabajo"); p != 4 {
		t.Errorf("after move, ResolveAlias(trabajo) = %d, want 4", p)
	}
	// clear
	a = a.ClearAlias(4)
	if a.AliasOf(4) != "" {
		t.Errorf("after clear, AliasOf(4) = %q", a.AliasOf(4))
	}
}

func TestResolveAliasDeterministicOnDuplicates(t *testing.T) {
	// A hand-edited file with case-variant duplicates resolves to the lowest profile.
	a := Aliases{3: "trabajo", 2: "Trabajo"}
	if p, ok := a.ResolveAlias("trabajo"); !ok || p != 2 {
		t.Errorf("ResolveAlias = (%d,%v), want (2,true)", p, ok)
	}
}

func TestLoadSaveRoundTripAndCorrupt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wa_aliases.json")

	// absent → empty, no error
	got, err := loadAliasesFrom(path)
	if err != nil || len(got) != 0 {
		t.Fatalf("loadAliasesFrom(absent) = (%v,%v), want ({} ,nil)", got, err)
	}

	// round trip
	a := Aliases{2: "trabajo"}
	if err := a.saveTo(path); err != nil {
		t.Fatalf("saveTo: %v", err)
	}
	got, err = loadAliasesFrom(path)
	if err != nil || got[2] != "trabajo" {
		t.Fatalf("round trip = (%v,%v)", got, err)
	}
	// no stray temp file left behind
	if _, statErr := osStat(path + ".tmp"); statErr == nil {
		t.Error("temp file was left behind")
	}

	// corrupt → error
	if err := writeFile(path, []byte("{not json")); err != nil {
		t.Fatal(err)
	}
	if _, err := loadAliasesFrom(path); err == nil {
		t.Error("loadAliasesFrom(corrupt) = nil error, want error")
	}
}

func TestResolveProfile(t *testing.T) {
	if n, err := ResolveProfile("2"); err != nil || n != 2 {
		t.Errorf(`ResolveProfile("2") = (%d,%v)`, n, err)
	}
	if _, err := ResolveProfile("5"); err == nil {
		t.Error(`ResolveProfile("5") should error`)
	}
	if _, err := ResolveProfile("ghost"); err == nil {
		t.Error(`ResolveProfile("ghost") should error (no such alias)`)
	}
}
```

Add small test helpers at the bottom of the test file so it compiles (these wrap stdlib so the round-trip test reads clearly):

```go
func osStat(p string) (interface{}, error) { return os.Stat(p) }
func writeFile(p string, b []byte) error   { return os.WriteFile(p, b, 0600) }
```

…and add `"os"` to the test imports.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/whatsapp/ -run 'Alias|ResolveProfile' -v`
Expected: FAIL — `ValidateAlias`, `Aliases`, `loadAliasesFrom`, `ResolveProfile`, etc. undefined (build error).

- [ ] **Step 3: Extract `configDir()` from `GetDBPath`**

In `internal/whatsapp/client.go`, pull the per-OS dir block out of `GetDBPath` into a helper, and have `GetDBPath` call it (no behavior change):

```go
// configDir returns the per-OS nikte config directory, creating it if needed.
func configDir() (string, error) {
	var dir string
	switch runtime.GOOS {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, "Library", "Application Support", "nikte")
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			appData = filepath.Join(home, "AppData", "Roaming")
		}
		dir = filepath.Join(appData, "nikte")
	default:
		configHome := os.Getenv("XDG_CONFIG_HOME")
		if configHome == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			configHome = filepath.Join(home, ".config")
		}
		dir = filepath.Join(configHome, "nikte")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return dir, nil
}
```

Replace the body of `GetDBPath` from `var configDir string` through the `MkdirAll` block with:

```go
func GetDBPath(profile int) (string, error) {
	name, err := dbFileName(profile)
	if err != nil {
		return "", err
	}
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}
```

- [ ] **Step 4: Create `internal/whatsapp/aliases.go`**

```go
package whatsapp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Aliases maps a profile number (1-4) to a human-friendly name.
type Aliases map[int]string

var aliasPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

// ValidateAlias checks an alias's format (not uniqueness). An alias must be
// shell-safe, not a bare number (ambiguous with 1-4), and not the reserved
// word "all" (used by `nk wa ls --all`).
func ValidateAlias(name string) error {
	name = strings.TrimSpace(name)
	if !aliasPattern.MatchString(name) {
		return fmt.Errorf("alias must be 1-32 chars of letters, digits, '-' or '_'")
	}
	if _, err := strconv.Atoi(name); err == nil {
		return fmt.Errorf("alias cannot be a number (those select profiles 1-4)")
	}
	if strings.EqualFold(name, "all") {
		return fmt.Errorf(`alias "all" is reserved`)
	}
	return nil
}

// AliasOf returns the alias for a profile, or "".
func (a Aliases) AliasOf(profile int) string { return a[profile] }

// ResolveAlias finds the profile whose alias matches name (case-insensitive,
// trimmed), scanning profiles 1→4 so the result is deterministic even if a
// hand-edited file contains case-variant duplicates.
func (a Aliases) ResolveAlias(name string) (int, bool) {
	name = strings.TrimSpace(name)
	for p := 1; p <= 4; p++ {
		if v, ok := a[p]; ok && strings.EqualFold(v, name) {
			return p, true
		}
	}
	return 0, false
}

// SetAlias assigns name to profile with move-semantics: it first removes name
// (case-insensitive) from any other profile, guaranteeing uniqueness.
func (a Aliases) SetAlias(profile int, name string) Aliases {
	if a == nil {
		a = Aliases{}
	}
	name = strings.TrimSpace(name)
	for p, v := range a {
		if strings.EqualFold(v, name) {
			delete(a, p)
		}
	}
	a[profile] = name
	return a
}

// ClearAlias removes the alias for a profile.
func (a Aliases) ClearAlias(profile int) Aliases {
	delete(a, profile)
	return a
}

func aliasFilePath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "wa_aliases.json"), nil
}

// loadAliasesFrom reads an alias map from path. Absent file → empty map, nil
// error. Present but unreadable/corrupt → error (so write callers can refuse to
// clobber it).
func loadAliasesFrom(path string) (Aliases, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Aliases{}, nil
	}
	if err != nil {
		return nil, err
	}
	var raw map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("wa_aliases.json is corrupt: %w", err)
	}
	out := Aliases{}
	for k, v := range raw {
		if p, err := strconv.Atoi(k); err == nil {
			out[p] = v
		}
	}
	return out, nil
}

// saveTo writes the alias map atomically (temp file + rename, mode 0600).
func (a Aliases) saveTo(path string) error {
	raw := map[string]string{}
	for p, v := range a {
		raw[strconv.Itoa(p)] = v
	}
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// LoadAliases reads the alias map from the default config location.
func LoadAliases() (Aliases, error) {
	path, err := aliasFilePath()
	if err != nil {
		return nil, err
	}
	return loadAliasesFrom(path)
}

// Save writes the alias map to the default config location.
func (a Aliases) Save() error {
	path, err := aliasFilePath()
	if err != nil {
		return err
	}
	return a.saveTo(path)
}

// ResolveProfile turns a raw -p value (a number 1-4 or a defined alias) into a
// profile number.
func ResolveProfile(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if n, err := strconv.Atoi(raw); err == nil {
		if err := ValidateProfile(n); err != nil {
			return 0, err
		}
		return n, nil
	}
	aliases, _ := LoadAliases() // tolerate a corrupt file on the read path
	if p, ok := aliases.ResolveAlias(raw); ok {
		return p, nil
	}
	return 0, fmt.Errorf("unknown profile %q (use 1-4 or a defined alias; see \"nk wa alias\")", raw)
}
```

- [ ] **Step 5: Run tests + build**

Run: `go test ./internal/whatsapp/ -v && make build`
Expected: PASS, `Built: nk-cli`.

- [ ] **Step 6: Commit**

```bash
git add internal/whatsapp/client.go internal/whatsapp/aliases.go internal/whatsapp/aliases_test.go
git commit -m "feat(wa): alias storage, validation, and profile resolution"
```

---

### Task 2: Migrate `-p` to StringP and resolve through every command

Change the `-p` flag to a string, resolve it (number or alias) in one helper, and replace all five flag reads. After this task, `-p 2` works exactly as before and `-p <alias>` resolves once aliases exist (Task 3 adds the command to create them). Output stays numeric for now.

**Files:**
- Modify: `internal/cli/wa.go`
- Test: `internal/cli/wa_test.go` (add a `profileFromCmd` round-trip test)

**Interfaces:**
- Consumes: `whatsapp.ResolveProfile(string) (int, error)` from Task 1.
- Produces: `func profileFromCmd(cmd *cobra.Command) (int, error)` — reads the `"profile"` string flag and resolves it.

- [ ] **Step 1: Write the failing test**

Add to `internal/cli/wa_test.go`:

```go
import "github.com/spf13/cobra"

func TestProfileFromCmd(t *testing.T) {
	c := &cobra.Command{}
	c.Flags().StringP("profile", "p", "1", "")
	if err := c.Flags().Set("profile", "2"); err != nil {
		t.Fatal(err)
	}
	n, err := profileFromCmd(c)
	if err != nil || n != 2 {
		t.Fatalf("profileFromCmd = (%d,%v), want (2,nil)", n, err)
	}
	if err := c.Flags().Set("profile", "9"); err != nil {
		t.Fatal(err)
	}
	if _, err := profileFromCmd(c); err == nil {
		t.Error("profileFromCmd(9) should error")
	}
}
```

(Keep the existing imports; add `"github.com/spf13/cobra"` if not already imported in the test file.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run TestProfileFromCmd -v`
Expected: FAIL — `profileFromCmd` undefined.

- [ ] **Step 3: Change the flag to StringP and add the helper**

In `internal/cli/wa.go`, change the registration at line ~132:

```go
	waCmd.PersistentFlags().StringP("profile", "p", "1", "WhatsApp profile: 1-4 or an alias")
```

Add the helper (near `notLinkedError`):

```go
// profileFromCmd reads the -p flag (a number 1-4 or an alias) and resolves it
// to a profile number.
func profileFromCmd(cmd *cobra.Command) (int, error) {
	raw, _ := cmd.Flags().GetString("profile")
	return whatsapp.ResolveProfile(raw)
}
```

Change the `PersistentPreRunE` (line ~135) to:

```go
	waCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		_, err := profileFromCmd(cmd)
		return err
	}
```

- [ ] **Step 4: Replace the four remaining flag reads**

In each of `runWaLink`, `runWaSend`, `runWaLs`, `runWaUnlink`, replace the first line
`profile, _ := cmd.Flags().GetInt("profile")` with:

```go
	profile, err := profileFromCmd(cmd)
	if err != nil {
		return err
	}
```

In `runWaSend`/`runWaLs`/`runWaUnlink` this `err` may shadow a later `err`; if the
function already declares `err` later with `:=`, change this one to assignment
form by declaring `profile` first:

```go
	profile, err := profileFromCmd(cmd)
	if err != nil {
		return err
	}
	_ = profile
```

(Use the simplest form the compiler accepts — `profile, err :=` then `if err != nil { return err }` — and fix any "declared and not used"/"no new variables" errors the compiler reports by switching `:=`/`=` as needed. Do not remove the error check.)

In `runWaStatus`, replace line ~773 `profile, _ := cmd.Flags().GetInt("profile")` with the same resolved form. Leave the `if !cmd.Flags().Changed("profile")` overview branch (line ~776) unchanged — `Changed` is type-independent and still distinguishes "no -p" from "-p given".

- [ ] **Step 5: Run tests + build + manual checks**

Run: `go test ./internal/cli/ -v && make build`
Expected: PASS, `Built: nk-cli`.

Manual:
- `./nk-cli wa status` → 4-profile overview (unchanged).
- `./nk-cli wa status -p 1` → detail for profile 1.
- `./nk-cli wa status -p 9` → error `profile must be between 1 and 4, got 9`.
- `./nk-cli wa status -p ghost` → error `unknown profile "ghost" (use 1-4 or a defined alias; see "nk wa alias")`.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/wa.go internal/cli/wa_test.go
git commit -m "feat(wa): -p accepts a number or alias (StringP + resolution)"
```

---

### Task 3: `nk wa alias` command and alias-aware output

Add the `nk wa alias` subcommand (list / set / clear) and switch profile-naming output to a `profileLabel` helper that shows the alias when one exists.

**Files:**
- Modify: `internal/cli/wa.go`
- Test: `internal/cli/wa_test.go` (add a `profileLabel` test)

**Interfaces:**
- Consumes: `whatsapp.LoadAliases`, `whatsapp.ValidateAlias`, `whatsapp.ResolveProfile`, `Aliases.SetAlias/ClearAlias/AliasOf/Save` from Task 1.
- Produces:
  - `func profileLabel(profile int) string` — alias if set, else the number as a string.
  - `runWaAlias` command wired under `waCmd` with its own no-op `PersistentPreRunE`.

- [ ] **Step 1: Write the failing test**

Add to `internal/cli/wa_test.go`:

```go
func TestProfileLabel(t *testing.T) {
	// No aliases configured in the test env → label is the number.
	if got := profileLabel(2); got != "2" {
		t.Errorf("profileLabel(2) = %q, want \"2\" when no alias set", got)
	}
}
```

(This verifies the fallback without depending on the real config dir. The alias
branch is exercised by the `whatsapp` package's `AliasOf` tests and the manual
smoke checks below.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run TestProfileLabel -v`
Expected: FAIL — `profileLabel` undefined.

- [ ] **Step 3: Add `profileLabel` and switch the naming output**

In `internal/cli/wa.go`, add:

```go
// profileLabel returns the alias for a profile if one is set, otherwise the
// profile number as a string. Used in user-facing messages.
func profileLabel(profile int) string {
	if aliases, err := whatsapp.LoadAliases(); err == nil {
		if name := aliases.AliasOf(profile); name != "" {
			return name
		}
	}
	return strconv.Itoa(profile)
}
```

(Ensure `"strconv"` is imported in wa.go.)

Update `notLinkedError` (wa.go:37-42) to use the label:

```go
func notLinkedError(profile int) error {
	label := profileLabel(profile)
	if profile == 1 && label == "1" {
		return fmt.Errorf(`WhatsApp not linked. Run "nk wa link" first`)
	}
	return fmt.Errorf(`WhatsApp profile %s not linked. Run "nk wa link -p %s" first`, label, label)
}
```

Update the `runWaStatus` detail branch lines that print `WhatsApp profile %d` / the
`-p %d` hint (wa.go:787-788, 798-799, 803) to use `profileLabel(profile)` with `%s`:

```go
		fmt.Printf("WhatsApp profile %s: Not linked\n", profileLabel(profile))
		fmt.Printf("Run \"nk wa link -p %s\" to connect this account.\n", profileLabel(profile))
```
(apply the same `%s`/`profileLabel` swap to the linked line `WhatsApp profile %s: Linked`).

Update `formatProfileLine` (wa.go:765) to show the alias in the overview row:

```go
// formatProfileLine renders one row of the multi-profile status overview,
// including the alias in parentheses when one is set.
func formatProfileLine(profile int, linked bool, id string) string {
	label := ""
	if aliases, err := whatsapp.LoadAliases(); err == nil {
		if name := aliases.AliasOf(profile); name != "" {
			label = " (" + name + ")"
		}
	}
	head := fmt.Sprintf("  %d%s", profile, label)
	if !linked {
		return head + "  Not linked"
	}
	return fmt.Sprintf("%s  Linked      (%s)", head, id)
}
```

- [ ] **Step 4: Add the `nk wa alias` subcommand**

In `addWaCommands`, define the command and register it under `waCmd` (alongside the existing `waCmd.AddCommand(...)`):

```go
	var aliasClear string
	aliasCmd := &cobra.Command{
		Use:   "alias [profile] [name]",
		Short: "Name a WhatsApp profile so -p can take the name",
		Long: `Manage profile aliases.

  nk wa alias                 List all profiles and their aliases
  nk wa alias 2 trabajo       Name profile 2 "trabajo" (then: nk wa send -p trabajo ...)
  nk wa alias 2 ""            Remove profile 2's alias
  nk wa alias --clear 2       Remove profile 2's alias

Aliases are 1-32 chars of letters, digits, '-' or '_', not a bare number, and
not "all". A name already used by another profile moves to the new one.`,
		Args: cobra.ArbitraryArgs,
		RunE: runWaAlias,
	}
	aliasCmd.Flags().StringVar(&aliasClear, "clear", "", "Remove the alias of the given profile")
	// Don't let an inherited, irrelevant -p block alias management.
	aliasCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error { return nil }

	waCmd.AddCommand(linkCmd, sendCmd, lsCmd, unlinkCmd, statusCmd, aliasCmd)
```

(Remove `aliasCmd` from being added twice — add it in the single existing `waCmd.AddCommand(...)` call.)

Add the runner:

```go
func runWaAlias(cmd *cobra.Command, args []string) error {
	aliases, err := whatsapp.LoadAliases()
	if err != nil {
		return err // refuse to clobber a corrupt file
	}

	clear, _ := cmd.Flags().GetString("clear")

	// `nk wa alias` with no args (and no --clear): list.
	if clear == "" && len(args) == 0 {
		fmt.Println("WhatsApp profiles:")
		for p := 1; p <= 4; p++ {
			name := aliases.AliasOf(p)
			if name == "" {
				name = "—"
			}
			fmt.Printf("  %d  %s\n", p, name)
		}
		return nil
	}

	// Determine the target profile (from --clear or the first positional arg).
	target := clear
	if target == "" {
		target = args[0]
	}
	profile, err := whatsapp.ResolveProfile(target)
	if err != nil {
		return err
	}

	// Clear: `--clear N`, or `alias N ""` (empty/whitespace name).
	name := ""
	if clear == "" && len(args) >= 2 {
		name = strings.TrimSpace(args[1])
	}
	if clear != "" || (len(args) >= 2 && name == "") {
		aliases = aliases.ClearAlias(profile)
		if err := aliases.Save(); err != nil {
			return err
		}
		fmt.Printf("Cleared alias for profile %d\n", profile)
		return nil
	}

	// Need a name to set.
	if len(args) < 2 {
		return fmt.Errorf("usage: nk wa alias <profile> <name>  (or --clear <profile>)")
	}
	if err := whatsapp.ValidateAlias(name); err != nil {
		return err
	}
	aliases = aliases.SetAlias(profile, name)
	if err := aliases.Save(); err != nil {
		return err
	}
	fmt.Printf("Profile %d is now \"%s\"\n", profile, name)
	return nil
}
```

(Ensure `"strings"` is imported in wa.go — it already is.)

- [ ] **Step 5: Run tests + build + manual smoke**

Run: `go test ./internal/... && make build`
Expected: PASS, `Built: nk-cli`.

Manual:
- `./nk-cli wa alias` → lists profiles 1-4 with `—`.
- `./nk-cli wa alias 2 trabajo` → `Profile 2 is now "trabajo"`.
- `./nk-cli wa alias` → profile 2 shows `trabajo`.
- `./nk-cli wa status` → row 2 shows `2 (trabajo)`.
- `./nk-cli wa send -p trabajo 0000000000 hi` → resolves to profile 2 (will report "not linked" as `profile trabajo not linked` if 2 isn't linked — that's expected).
- `./nk-cli wa alias 2 ""` → `Cleared alias for profile 2`.
- `./nk-cli wa alias 2 "bad name"` → format error.
- `./nk-cli wa -p whatever alias` → still lists (inherited -p doesn't block alias).

- [ ] **Step 6: Commit**

```bash
git add internal/cli/wa.go internal/cli/wa_test.go
git commit -m "feat(wa): nk wa alias command + alias-aware status/errors"
```

---

## Self-Review

**Spec coverage:**
- `-p` accepts number or alias → Task 2 (`profileFromCmd`/`ResolveProfile`). ✓
- Alias format rules (`[A-Za-z0-9_-]{1,32}`, not numeric, not `all`) → Task 1 `ValidateAlias`. ✓
- Move-semantics / uniqueness postcondition → Task 1 `SetAlias`. ✓
- Deterministic resolution → Task 1 `ResolveAlias` (1→4 scan). ✓
- Absent→empty / corrupt→error; reads tolerate, write aborts → Task 1 `loadAliasesFrom` + Task 3 `runWaAlias` (returns the load error). ✓
- Atomic write → Task 1 `saveTo` (temp + rename, 0600). ✓
- Empty name = remove → Task 3 `runWaAlias` clear branch. ✓
- Aliasing without a linked DB → `ResolveProfile`/`SetAlias` need only `ValidateProfile`, never a DB (Task 1/3). ✓
- `nk wa alias` list/set/clear → Task 3. ✓
- Alias-aware output (status rows, notLinkedError, status detail) → Task 3 `profileLabel`/`formatProfileLine`. ✓
- StringP migration replaces all 5 reads; `Changed` routing preserved → Task 2. ✓
- `aliasCmd` not blocked by inherited `-p` → Task 3 no-op `PersistentPreRunE`. ✓

**Releasability:** Task 1 adds package code (no CLI change). Task 2 keeps `-p N` identical and makes `-p alias` resolve (no alias yet → clean "unknown profile" error). Task 3 adds the command + labels. Every commit builds and is releasable.

**Placeholder scan:** none — all steps carry concrete code/commands. The one judgment call (resolving `:=`/`=` shadowing in Task 2 Step 4) names the exact compiler errors to fix and forbids removing the error check.

**Type consistency:** `Aliases map[int]string`; `ResolveProfile(string)(int,error)`, `ValidateAlias(string)error`, `profileFromCmd(*cobra.Command)(int,error)`, `profileLabel(int)string` used consistently across tasks; `loadAliasesFrom`/`saveTo` (path-parameterized, tested) vs `LoadAliases`/`Save` (default-path wrappers) match their call sites.
