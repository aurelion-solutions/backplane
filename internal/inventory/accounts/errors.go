// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package accounts

import "errors"

// ErrInvalidPayload is returned when a lake record is missing the
// required fields (application_id, username).
var ErrInvalidPayload = errors.New("accounts: invalid payload")
