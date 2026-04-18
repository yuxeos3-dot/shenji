package api

import "testing"

// #42: Role validation gate. UpdateUser rejects anything outside the
// documented enum so a malformed request can't wedge a user row into a
// state that bypasses the admin-count guard (e.g. "admin2" would look
// like "not admin" to the demotion check but wouldn't trigger it on
// Delete either).
func TestIsValidUserRole(t *testing.T) {
	valid := []string{"", "admin", "member"}
	for _, r := range valid {
		if !isValidUserRole(r) {
			t.Errorf("expected %q to be valid", r)
		}
	}
	invalid := []string{
		"Admin",      // case sensitive
		"admin ",     // trailing whitespace
		"ADMIN",      // caps
		"admin2",     // lookalike
		"root",       // unknown role
		"superadmin", // unknown role
		"guest",      // unknown role
		"*",
	}
	for _, r := range invalid {
		if isValidUserRole(r) {
			t.Errorf("expected %q to be rejected", r)
		}
	}
}
