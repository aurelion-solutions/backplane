// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package grammar

import "testing"

func TestEmbedded_Compiles(t *testing.T) {
	if _, err := Compiled(); err != nil {
		t.Fatalf("Compiled: %v", err)
	}
}

func TestEmbedded_Parses(t *testing.T) {
	doc, err := Parsed()
	if err != nil {
		t.Fatalf("Parsed: %v", err)
	}
	if doc["$id"] != SchemaURL {
		t.Fatalf("$id = %v, want %v", doc["$id"], SchemaURL)
	}
}
