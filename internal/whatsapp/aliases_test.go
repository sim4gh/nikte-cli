package whatsapp

import (
	"os"
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
		"-x", "-bad", "_lead", "this-name-is-way-too-long-to-be-valid-x"}
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
	// Numeric resolution never reads the alias file, so these stay hermetic.
	if n, err := ResolveProfile("2"); err != nil || n != 2 {
		t.Errorf(`ResolveProfile("2") = (%d,%v)`, n, err)
	}
	if _, err := ResolveProfile("5"); err == nil {
		t.Error(`ResolveProfile("5") should error`)
	}
	// Alias resolution is tested against an in-memory map so the test never
	// touches the real config dir.
	if p, err := resolveAlias("trabajo", Aliases{2: "trabajo"}); err != nil || p != 2 {
		t.Errorf(`resolveAlias("trabajo") = (%d,%v), want (2,nil)`, p, err)
	}
	if _, err := resolveAlias("ghost", Aliases{}); err == nil {
		t.Error(`resolveAlias("ghost") should error (no such alias)`)
	}
}

func TestSaveToCleansUpTempOnError(t *testing.T) {
	dir := t.TempDir()
	// Make the destination a directory so os.Rename(tmp, path) fails, exercising
	// the error path that must clean up the temp file.
	path := filepath.Join(dir, "dest")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := (Aliases{2: "x"}).saveTo(path); err == nil {
		t.Fatal("saveTo onto a directory should fail")
	}
	if _, err := osStat(path + ".tmp"); err == nil {
		t.Error("stray .tmp left behind after a failed saveTo")
	}
}

func osStat(p string) (interface{}, error) { return os.Stat(p) }
func writeFile(p string, b []byte) error   { return os.WriteFile(p, b, 0600) }
