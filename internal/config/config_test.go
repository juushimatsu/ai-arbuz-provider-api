package config

import "testing"

// audit #6: weak/placeholder/empty admin passwords must be rejected.
func TestValidateAdminPassword(t *testing.T) {
	bad := []string{"", "short", "changeme", "ChangeMe", "password", "admin"}
	for _, p := range bad {
		if err := validateAdminPassword(p); err == nil {
			t.Errorf("expected rejection for %q", p)
		}
	}
	if err := validateAdminPassword("StrongPass123"); err != nil {
		t.Errorf("expected acceptance, got %v", err)
	}
}
