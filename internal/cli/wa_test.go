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
