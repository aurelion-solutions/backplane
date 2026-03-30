// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package shared holds vocabularies and constants used across the
// inventory slices — PrincipalKind, status enums for the principal kinds
// that carry an IGA-universal vocabulary, and event routing-key
// prefixes. Keeping them here avoids cycles between the per-entity
// packages (principals ↔ employments ↔ workloads ↔ customers) when one
// needs to reference another's vocabulary.
package shared

// PrincipalKind is the discriminator over a Principal's body type.
//
// Employment, not Person, is the principal: one human can hold several
// concurrent employments (e.g. a developer mask and a part-time QA
// mask on the same legal entity); each mask gets its own Principal
// row and its own access posture.
//
// Workload and Customer are first-class principal kinds independent of
// human identity.
type PrincipalKind string

const (
	PrincipalKindEmployment PrincipalKind = "employment"
	PrincipalKindWorkload   PrincipalKind = "workload"
	PrincipalKindCustomer   PrincipalKind = "customer"
)

// Valid reports whether k matches a defined PrincipalKind.
func (k PrincipalKind) Valid() bool {
	switch k {
	case PrincipalKindEmployment, PrincipalKindWorkload, PrincipalKindCustomer:
		return true
	}
	return false
}

// WorkloadStatus enumerates the lifecycle statuses derived for a
// workload principal. Workload status vocabulary is IGA-universal —
// every tenant uses the same three values.
type WorkloadStatus string

const (
	WorkloadStatusActive  WorkloadStatus = "active"
	WorkloadStatusExpired WorkloadStatus = "expired"
	WorkloadStatusLocked  WorkloadStatus = "locked"
)

// Valid reports whether s is a recognised WorkloadStatus.
func (s WorkloadStatus) Valid() bool {
	switch s {
	case WorkloadStatusActive, WorkloadStatusExpired, WorkloadStatusLocked:
		return true
	}
	return false
}

// CustomerStatus enumerates the lifecycle statuses derived for a
// customer principal. Mirrors the kernel SubjectCustomerStatus
// vocabulary; product-level, not tenant-customisable.
type CustomerStatus string

const (
	CustomerStatusRegistered        CustomerStatus = "registered"
	CustomerStatusVerified          CustomerStatus = "verified"
	CustomerStatusActive            CustomerStatus = "active"
	CustomerStatusSuspended         CustomerStatus = "suspended"
	CustomerStatusBanned            CustomerStatus = "banned"
	CustomerStatusDeletionRequested CustomerStatus = "deletion_requested"
)

// Valid reports whether s is a recognised CustomerStatus.
func (s CustomerStatus) Valid() bool {
	switch s {
	case CustomerStatusRegistered, CustomerStatusVerified, CustomerStatusActive,
		CustomerStatusSuspended, CustomerStatusBanned, CustomerStatusDeletionRequested:
		return true
	}
	return false
}

// StatusForKind reports whether status is acceptable for the given
// PrincipalKind. Employment status is tenant-defined (every company
// names their working states differently — "active", "probation",
// "maternity_leave", "notice_period", "sabbatical" — and we will not
// pretend otherwise), so any non-empty 64-char-or-less string passes.
// Workload and Customer remain bound to their universal vocabularies.
func StatusForKind(kind PrincipalKind, status string) bool {
	switch kind {
	case PrincipalKindEmployment:
		return status != "" && len(status) <= 64
	case PrincipalKindWorkload:
		return WorkloadStatus(status).Valid()
	case PrincipalKindCustomer:
		return CustomerStatus(status).Valid()
	}
	return false
}

// CustomerTenantRole enumerates the role a Customer holds within their
// tenant. Optional column on the Customer model (nullable).
type CustomerTenantRole string

const (
	CustomerTenantRoleAdmin  CustomerTenantRole = "admin"
	CustomerTenantRoleMember CustomerTenantRole = "member"
	CustomerTenantRoleViewer CustomerTenantRole = "viewer"
)

// Valid reports whether r is a recognised CustomerTenantRole.
func (r CustomerTenantRole) Valid() bool {
	switch r {
	case CustomerTenantRoleAdmin, CustomerTenantRoleMember, CustomerTenantRoleViewer:
		return true
	}
	return false
}

// CustomerPlanTier enumerates the billing plan a Customer is on.
type CustomerPlanTier string

const (
	CustomerPlanTierFree       CustomerPlanTier = "free"
	CustomerPlanTierBasic      CustomerPlanTier = "basic"
	CustomerPlanTierPro        CustomerPlanTier = "pro"
	CustomerPlanTierEnterprise CustomerPlanTier = "enterprise"
)

// Valid reports whether t is a recognised CustomerPlanTier.
func (t CustomerPlanTier) Valid() bool {
	switch t {
	case CustomerPlanTierFree, CustomerPlanTierBasic, CustomerPlanTierPro, CustomerPlanTierEnterprise:
		return true
	}
	return false
}
