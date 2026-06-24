package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

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

func TestNotLinkedError(t *testing.T) {
	if got := notLinkedError(1).Error(); got != `WhatsApp not linked. Run "nk wa link" first` {
		t.Errorf("profile 1 message = %q", got)
	}
	got := notLinkedError(2).Error()
	if !strings.Contains(got, "profile 2") || !strings.Contains(got, `nk wa link -p 2`) {
		t.Errorf("profile 2 message = %q, want it to mention profile 2 and -p 2", got)
	}
}

func TestProfileLabel(t *testing.T) {
	// No aliases configured in the test env → label is the number.
	if got := profileLabel(2); got != "2" {
		t.Errorf("profileLabel(2) = %q, want \"2\" when no alias set", got)
	}
}

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
