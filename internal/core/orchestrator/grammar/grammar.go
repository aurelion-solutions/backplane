// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package grammar exposes the embedded pipeline YAML JSON Schema as
// the single source of truth.
//
// Both the loader (Step 2) and the well-known endpoint (Step 5) read
// from here — no parallel schema definitions allowed elsewhere.
package grammar

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

//go:embed schema.json
var rawSchema []byte

var (
	once       sync.Once
	compiled   *jsonschema.Schema
	compileErr error
)

// SchemaURL is the JSON Schema $id used to register the embedded
// document with the jsonschema compiler.
const SchemaURL = "https://aurelion.dev/schemas/pipeline.schema.json"

// Bytes returns the verbatim embedded schema.json source.
//
// Callers that need to render the schema (e.g. the well-known endpoint)
// should deep-copy the parsed form via Parsed instead — this byte
// slice is for hashing / debugging only.
func Bytes() []byte {
	return rawSchema
}

// Parsed returns the embedded schema decoded into a fresh
// map[string]any (callers can mutate safely).
func Parsed() (map[string]any, error) {
	var out map[string]any
	if err := json.Unmarshal(rawSchema, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Compiled returns the compiled validator, building it lazily on first
// call. Safe for concurrent use.
func Compiled() (*jsonschema.Schema, error) {
	once.Do(func() {
		c := jsonschema.NewCompiler()
		c.Draft = jsonschema.Draft2020
		if err := c.AddResource(SchemaURL, bytes.NewReader(rawSchema)); err != nil {
			compileErr = err
			return
		}
		compiled, compileErr = c.Compile(SchemaURL)
	})
	return compiled, compileErr
}
