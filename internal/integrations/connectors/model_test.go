// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package connectors

import (
	"testing"
	"time"
)

func TestIsOnlineAt(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name      string
		lastSeen  time.Time
		wantAlive bool
	}{
		{"just now", now, true},
		{"30 seconds ago", now.Add(-30 * time.Second), true},
		{"just inside window", now.Add(-onlineThreshold + time.Second), true},
		{"exactly at threshold", now.Add(-onlineThreshold), true},
		{"one tick past threshold", now.Add(-onlineThreshold - time.Nanosecond), false},
		{"5 minutes ago", now.Add(-5 * time.Minute), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inst := &ConnectorInstance{LastSeenAt: tc.lastSeen}
			if got := inst.IsOnlineAt(now); got != tc.wantAlive {
				t.Fatalf("want %v, got %v", tc.wantAlive, got)
			}
		})
	}
}
