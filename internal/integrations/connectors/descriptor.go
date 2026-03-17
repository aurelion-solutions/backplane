// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package connectors

// CapabilityDescriptor is the structured advertisement a connector
// sends on registration: which operations it supports, which fact
// kinds it can verify, allowed account-status transitions, and the
// cascade rules it expects the orchestrator to apply.
//
// JSON tags match kernel's ConnectorCapabilityDescriptor — engines on
// either side of the wire read the same shape.
type CapabilityDescriptor struct {
	Operations          []OperationDescriptor    `json:"operations,omitempty"`
	AccountStatus       AccountStatusTransitions `json:"account_status"`
	VerifyFactSupported bool                     `json:"verify_fact_supported"`
	SupportedFactKinds  []string                 `json:"supported_fact_kinds,omitempty"`
	Cascades            AccountDisableCascades   `json:"cascades"`
}

// OperationDescriptor describes one operation a connector can execute.
type OperationDescriptor struct {
	Kind            string                    `json:"kind"`
	DependencyRules []OperationDependencyRule `json:"dependency_rules,omitempty"`
}

// OperationDependencyRule says "this resource must already exist in the
// given status before the parent operation runs". Application is
// optional — when set the dependency lives in a different application.
type OperationDependencyRule struct {
	Resource    string   `json:"resource"`
	Status      []string `json:"status"`
	Application *string  `json:"application,omitempty"`
}

// AccountStatusTransitions enumerates the legal (from, to) pairs the
// connector accepts on accounts it owns. Encoded as 2-element arrays to
// match kernel's tuple shape on the wire.
type AccountStatusTransitions struct {
	Transitions [][2]string `json:"transitions,omitempty"`
}

// AccountDisableCascades lists fact kinds that must be revoked before
// the connector executes account_disable.
type AccountDisableCascades struct {
	BeforeDisable []AccountDisableCascadeRule `json:"before_disable,omitempty"`
}

// AccountDisableCascadeRule names one fact kind to revoke.
type AccountDisableCascadeRule struct {
	FactKind string `json:"fact_kind"`
}
