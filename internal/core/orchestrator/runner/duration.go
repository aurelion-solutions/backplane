// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package runner

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// durationRe mirrors the grammar pattern ^\d+(s|m|h|d)$.
var durationRe = regexp.MustCompile(`^(\d+)(s|m|h|d)$`)

// parseDuration converts the grammar's "30s" / "1h" / "7d" forms into
// a Go time.Duration. Returns an error on malformed input.
func parseDuration(s string) (time.Duration, error) {
	m := durationRe.FindStringSubmatch(s)
	if m == nil {
		return 0, fmt.Errorf("runner: invalid duration %q (want ^\\d+(s|m|h|d)$)", s)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, err
	}
	switch m[2] {
	case "s":
		return time.Duration(n) * time.Second, nil
	case "m":
		return time.Duration(n) * time.Minute, nil
	case "h":
		return time.Duration(n) * time.Hour, nil
	case "d":
		return time.Duration(n) * 24 * time.Hour, nil
	}
	return 0, fmt.Errorf("runner: unreachable")
}
