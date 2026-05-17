// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package authzen

import "errors"

var (
	errSubjectMissing      = errors.New("authzen: subject.type and subject.id are required")
	errResourceTypeMissing = errors.New("authzen: resource.type is required")
	errActionMissing       = errors.New("authzen: action.name is required")
)
