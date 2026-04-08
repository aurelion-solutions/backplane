// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package beat

import (
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/robfig/cron/v3"
)

// PreviousFirePoint returns the most recent fire point that should
// have fired at or before now for the given schedule.
//
// Exactly one of cron or every must be non-empty.
//
// For "every": uses an epoch-anchored floor (Unix epoch in UTC) so
// the result is deterministic across restarts and replicas.
//
// For "cron": uses robfig/cron with the standard 5-field parser.
func PreviousFirePoint(now time.Time, cronExpr, every string) (time.Time, error) {
	if (cronExpr == "") == (every == "") {
		return time.Time{}, fmt.Errorf("beat: exactly one of cron or every required")
	}

	nowUTC := now.UTC()

	if every != "" {
		d, err := parseDuration(every)
		if err != nil {
			return time.Time{}, fmt.Errorf("beat: invalid every %q: %w", every, err)
		}
		secs := int64(d / time.Second)
		if secs <= 0 {
			return time.Time{}, fmt.Errorf("beat: every must be >= 1s")
		}
		elapsed := nowUTC.Unix()
		floored := (elapsed / secs) * secs
		return time.Unix(floored, 0).UTC(), nil
	}

	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	sched, err := parser.Parse(cronExpr)
	if err != nil {
		return time.Time{}, fmt.Errorf("beat: invalid cron %q: %w", cronExpr, err)
	}
	// robfig/cron returns the NEXT occurrence; to get the previous one
	// we step backwards by querying Next from a point in the past and
	// walking forward until we pass now.
	candidate := nowUTC.Add(-time.Hour * 24 * 366) // 1 year back
	var prev time.Time
	for {
		next := sched.Next(candidate)
		if next.After(nowUTC) {
			return prev, nil
		}
		prev = next
		candidate = next
	}
}

var durationRe = regexp.MustCompile(`^(\d+)(s|m|h|d)$`)

func parseDuration(s string) (time.Duration, error) {
	m := durationRe.FindStringSubmatch(s)
	if m == nil {
		return 0, fmt.Errorf("beat: invalid duration %q", s)
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
	return 0, fmt.Errorf("beat: unreachable")
}
