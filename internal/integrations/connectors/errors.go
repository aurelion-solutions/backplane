// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package connectors

import "errors"

// ErrInstanceNotFound is returned when a ConnectorInstance with the
// requested instance_id (or uuid) is not registered.
var ErrInstanceNotFound = errors.New("connectors: instance not found")

// ErrNoMatchingInstance is returned when no online instance satisfies
// the supplied required-tag set.
var ErrNoMatchingInstance = errors.New("connectors: no online instance matches required tags")

// ErrRPCStatus wraps a non-ok status returned by the remote connector.
// Callers can errors.As it to read the status and remote message.
type ErrRPCStatus struct {
	Status  string
	Message string
}

func (e *ErrRPCStatus) Error() string {
	if e.Status == "" {
		return "connectors: rpc returned error: " + e.Message
	}
	return "connectors: rpc returned status=" + e.Status + ": " + e.Message
}
