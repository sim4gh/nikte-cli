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
