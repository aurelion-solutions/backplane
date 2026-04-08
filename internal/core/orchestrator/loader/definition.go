// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package loader

// Definition is the immutable in-memory projection of one validated
// pipeline YAML. Built exclusively by Loader — never construct
// directly in application code.
//
// Triggers and Steps are kept as raw map[string]any so the runner and
// matcher can inspect arbitrary nested fields (mq match predicates,
// templated args) without forcing every grammar evolution through Go
// struct changes.
type Definition struct {
	Name          string           `json:"name"`
	Version       int              `json:"version"`
	SchemaVersion int              `json:"schema_version"`
	SourcePath    string           `json:"source_path"`
	ContentHash   string           `json:"content_hash"`
	ArgsSchema    map[string]any   `json:"args_schema"`
	Triggers      []map[string]any `json:"triggers"`
	Steps         []map[string]any `json:"steps"`
	Raw           map[string]any   `json:"-"`
}

// Step kinds — string literals lifted from the grammar.
const (
	StepWaitForEvent = "wait_for_event"
)

// StepKind returns the discriminator for a step entry:
//
//	"wait_for_event" — when type == "wait_for_event"
//	"engine_call"    — otherwise (the grammar accepts engine+action only)
func StepKind(step map[string]any) string {
	if t, _ := step["type"].(string); t == StepWaitForEvent {
		return StepWaitForEvent
	}
	return "engine_call"
}

// StepName returns the step name (assumed valid after schema check).
func StepName(step map[string]any) string {
	n, _ := step["name"].(string)
	return n
}

// StepRequires returns the requires[] array (assumed valid after
// schema check). Returns nil when absent.
func StepRequires(step map[string]any) []string {
	v, ok := step["requires"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(v))
	for _, item := range v {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
