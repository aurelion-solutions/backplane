// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package shared

import "testing"

func TestPrincipalKind_Valid(t *testing.T) {
	for _, k := range []PrincipalKind{PrincipalKindEmployment, PrincipalKindWorkload, PrincipalKindCustomer} {
		if !k.Valid() {
			t.Fatalf("expected %q to be valid", k)
		}
	}
	for _, bad := range []PrincipalKind{"", "employee", "person", "EMPLOYMENT"} {
		if bad.Valid() {
			t.Fatalf("expected %q to be invalid", bad)
		}
	}
}

func TestStatusForKind_Employment_acceptsAnyString(t *testing.T) {
	for _, s := range []string{"active", "probation", "maternity_leave", "notice_period", "sabbatical", "x"} {
		if !StatusForKind(PrincipalKindEmployment, s) {
			t.Fatalf("StatusForKind(employment, %q) = false, want true (tenant-defined vocabulary)", s)
		}
	}
	// Empty and too-long strings are still rejected.
	if StatusForKind(PrincipalKindEmployment, "") {
		t.Fatal("empty status should be invalid")
	}
	long := make([]byte, 65)
	for i := range long {
		long[i] = 'x'
	}
	if StatusForKind(PrincipalKindEmployment, string(long)) {
		t.Fatal("status > 64 chars should be invalid")
	}
}

func TestStatusForKind_WorkloadAndCustomer(t *testing.T) {
	cases := []struct {
		kind   PrincipalKind
		status string
		want   bool
	}{
		{PrincipalKindWorkload, "active", true},
		{PrincipalKindWorkload, "expired", true},
		{PrincipalKindWorkload, "locked", true},
		{PrincipalKindWorkload, "probation", false},
		{PrincipalKindWorkload, "", false},

		{PrincipalKindCustomer, "registered", true},
		{PrincipalKindCustomer, "verified", true},
		{PrincipalKindCustomer, "active", true},
		{PrincipalKindCustomer, "suspended", true},
		{PrincipalKindCustomer, "banned", true},
		{PrincipalKindCustomer, "deletion_requested", true},
		{PrincipalKindCustomer, "expired", false},

		{PrincipalKind("unknown"), "active", false},
	}
	for _, c := range cases {
		if got := StatusForKind(c.kind, c.status); got != c.want {
			t.Fatalf("StatusForKind(%q, %q) = %v, want %v", c.kind, c.status, got, c.want)
		}
	}
}

func TestCustomerTenantRole_Valid(t *testing.T) {
	for _, r := range []CustomerTenantRole{CustomerTenantRoleAdmin, CustomerTenantRoleMember, CustomerTenantRoleViewer} {
		if !r.Valid() {
			t.Fatalf("expected %q valid", r)
		}
	}
	for _, bad := range []CustomerTenantRole{"", "owner", "ADMIN"} {
		if bad.Valid() {
			t.Fatalf("expected %q invalid", bad)
		}
	}
}

func TestCustomerPlanTier_Valid(t *testing.T) {
	for _, p := range []CustomerPlanTier{CustomerPlanTierFree, CustomerPlanTierBasic, CustomerPlanTierPro, CustomerPlanTierEnterprise} {
		if !p.Valid() {
			t.Fatalf("expected %q valid", p)
		}
	}
	for _, bad := range []CustomerPlanTier{"", "premium", "FREE"} {
		if bad.Valid() {
			t.Fatalf("expected %q invalid", bad)
		}
	}
}
