// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package orchestrator

import "testing"

func TestContentHash_KeyOrderIndependent(t *testing.T) {
	a := ContentHash(map[string]any{"x": 1, "y": "two"})
	b := ContentHash(map[string]any{"y": "two", "x": 1})
	if a != b {
		t.Fatalf("hash differs by key order: %s vs %s", a, b)
	}
}

func TestContentHash_ChangesOnValueChange(t *testing.T) {
	a := ContentHash(map[string]any{"x": 1})
	b := ContentHash(map[string]any{"x": 2})
	if a == b {
		t.Fatalf("hash unchanged across values: %s", a)
	}
}

func TestContentHash_HexLength(t *testing.T) {
	if h := ContentHash(map[string]any{}); len(h) != 64 {
		t.Fatalf("hash len = %d, want 64", len(h))
	}
}
