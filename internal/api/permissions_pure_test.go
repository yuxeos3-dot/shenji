package api

import (
	"strings"
	"testing"
)

// cleanPermissionList is the single guardrail protecting against arbitrary
// permission codes being persisted to users.permissions. If an unknown
// code slipped through, a malicious payload could later be tested against
// HasPermission and silently succeed if anyone ever loosened the matcher.
// These tests ensure the whitelist semantics, dedup, and error reporting
// all behave as advertised.
func TestCleanPermissionList(t *testing.T) {
	t.Run("all_valid_preserved", func(t *testing.T) {
		in := []string{"sites.view", "dashboard.view"}
		out, err := cleanPermissionList(in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(out) != 2 {
			t.Fatalf("expected 2, got %d: %v", len(out), out)
		}
	})

	t.Run("duplicates_removed", func(t *testing.T) {
		in := []string{"sites.view", "sites.view", "dashboard.view", "sites.view"}
		out, err := cleanPermissionList(in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(out) != 2 {
			t.Fatalf("expected 2 unique entries, got %d: %v", len(out), out)
		}
	})

	t.Run("unknown_rejected", func(t *testing.T) {
		in := []string{"sites.view", "not.a.real.permission"}
		out, err := cleanPermissionList(in)
		if err == nil {
			t.Fatalf("expected error, got %v", out)
		}
		if !strings.Contains(err.Error(), "not.a.real.permission") {
			t.Fatalf("error should name the offending code, got %q", err.Error())
		}
	})

	t.Run("empty_input_ok", func(t *testing.T) {
		out, err := cleanPermissionList(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(out) != 0 {
			t.Fatalf("expected empty, got %v", out)
		}
	})

	t.Run("privilege_escalation_attempt", func(t *testing.T) {
		// A payload crafted to look like a wildcard or admin bypass must
		// be rejected — we don't support wildcards, and HasPermission
		// uses an exact match.
		for _, code := range []string{"*", "admin", "sites.*", "", "sites.view "} {
			if _, err := cleanPermissionList([]string{code}); err == nil {
				t.Errorf("expected rejection for %q", code)
			}
		}
	})
}

// knownPermCodes underpins cleanPermissionList. If the tree is ever
// emptied by accident, every permission submit would fail silently —
// assert a handful of well-known codes stay reachable.
func TestKnownPermCodesContainsCoreCodes(t *testing.T) {
	known := knownPermCodes()
	for _, code := range []string{
		"dashboard.view",
		"sites.view",
		"coord.pre_audit",
		"ai_gen.submit",
	} {
		if _, ok := known[code]; !ok {
			t.Errorf("expected %q to be in permission tree", code)
		}
	}
}
