package accountimport

import "testing"

func TestSafeDisplayNameNeverUsesEmail(t *testing.T) {
	got := SafeDisplayName("alice@seller.example", "desk-9", "acct-9", 3)
	if got == "alice@seller.example" {
		t.Fatal("email used as display name")
	}
	if got != "ChatGPT desk-9" {
		t.Fatalf("got %q", got)
	}

	got = SafeDisplayName("门店A-1", "desk-9", "", 1)
	if got != "门店A-1" {
		t.Fatalf("alias = %q", got)
	}
}
