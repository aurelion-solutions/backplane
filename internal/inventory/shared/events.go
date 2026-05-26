// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package shared

// Routing-key constants for every inventory event emitted by the
// inventory slices. Grammar is <domain>.<entity>.<operation>; matches
// the kernel routing-key shape so existing consumers keep working.
//
// Catalogued here, not per-slice, because:
//   - several events are emitted across slice boundaries
//     (e.g. principals emits inventory.principal.status_recomputed while
//     the trigger sits in customers / workloads / employments),
//   - keeping the catalog central makes it impossible to silently
//     diverge between producer and consumer.
const (
	EventActorComponentPersons         = "inventory.persons"
	EventActorComponentOrgUnits        = "inventory.org_units"
	EventActorComponentEmployments     = "inventory.employments"
	EventActorComponentEmployeeRecords = "inventory.employee_records"
	EventActorComponentWorkloads       = "inventory.workloads"
	EventActorComponentCustomers       = "inventory.customers"
	EventActorComponentPrincipals      = "inventory.principals"
)

// Person events.
const (
	EventPersonCreated          = "inventory.person.created"
	EventPersonBulkUpserted     = "inventory.person.bulk_upserted"
	EventPersonAttributeAdded   = "inventory.person.attribute_added"
	EventPersonAttributeRemoved = "inventory.person.attribute_removed"
)

// OrgUnit events.
const (
	EventOrgUnitCreated      = "inventory.org_unit.created"
	EventOrgUnitUpdated      = "inventory.org_unit.updated"
	EventOrgUnitDeleted      = "inventory.org_unit.deleted"
	EventOrgUnitBulkUpserted = "inventory.org_unit.bulk_upserted"
)

// Employment events.
const (
	EventEmploymentCreated          = "inventory.employment.created"
	EventEmploymentUpdated          = "inventory.employment.updated"
	EventEmploymentEnded            = "inventory.employment.ended"
	EventEmploymentBulkUpserted     = "inventory.employment.bulk_upserted"
	EventEmploymentAttributeAdded   = "inventory.employment.attribute_added"
	EventEmploymentAttributeRemoved = "inventory.employment.attribute_removed"
)

// EmployeeRecord events.
const (
	EventEmployeeRecordCreated          = "inventory.employee_record.created"
	EventEmployeeRecordBulkUpserted     = "inventory.employee_record.bulk_upserted"
	EventEmployeeRecordAttributeAdded   = "inventory.employee_record.attribute_added"
	EventEmployeeRecordAttributeRemoved = "inventory.employee_record.attribute_removed"
	EventEmployeeRecordMatched          = "inventory.employee_record.matched"
	EventEmployeeRecordUnmatched        = "inventory.employee_record.unmatched"
)

// Workload events.
const (
	EventWorkloadCreated          = "inventory.workload.created"
	EventWorkloadUpdated          = "inventory.workload.updated"
	EventWorkloadExpired          = "inventory.workload.expired"
	EventWorkloadBulkUpserted     = "inventory.workload.bulk_upserted"
	EventWorkloadAttributeAdded   = "inventory.workload.attribute_added"
	EventWorkloadAttributeRemoved = "inventory.workload.attribute_removed"
)

// Customer events.
const (
	EventCustomerCreated          = "inventory.customer.created"
	EventCustomerUpdated          = "inventory.customer.updated"
	EventCustomerBulkUpserted     = "inventory.customer.bulk_upserted"
	EventCustomerAttributeAdded   = "inventory.customer.attribute_added"
	EventCustomerAttributeRemoved = "inventory.customer.attribute_removed"
)

// Principal events.
const (
	EventPrincipalCreated          = "inventory.principal.created"
	EventPrincipalStatusRecomputed = "inventory.principal.status_recomputed"
	EventPrincipalLocked           = "inventory.principal.locked"
	EventPrincipalUnlocked         = "inventory.principal.unlocked"
)
