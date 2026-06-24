package cli

import (
	"strings"
	"testing"
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
