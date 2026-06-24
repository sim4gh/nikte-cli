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

// First char must be alphanumeric so an alias can never look like a flag
// (e.g. "-x"), keeping generated command suggestions like `nk wa link -p <alias>`
// shell-safe.
var aliasPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,31}$`)

// ValidateAlias checks an alias's format (not uniqueness). An alias must be
// shell-safe, not a bare number (ambiguous with 1-4), and not the reserved
// word "all" (used by `nk wa ls --all`).
func ValidateAlias(name string) error {
	name = strings.TrimSpace(name)
	if !aliasPattern.MatchString(name) {
		return fmt.Errorf("alias must be 1-32 chars of letters, digits, '-' or '_', starting with a letter or digit")
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
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
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
	return resolveAlias(raw, aliases)
}

// resolveAlias resolves a non-numeric -p value against the given alias map.
// Split out from ResolveProfile so it can be tested without reading the real
// config directory.
func resolveAlias(raw string, aliases Aliases) (int, error) {
	if p, ok := aliases.ResolveAlias(raw); ok {
		return p, nil
	}
	return 0, fmt.Errorf("unknown profile %q (use 1-4 or a defined alias; see \"nk wa alias\")", raw)
}
