// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package loader

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/grammar"
	"github.com/santhosh-tekuri/jsonschema/v5"
	"gopkg.in/yaml.v3"
)

// ActionLookup is the contract the loader uses to verify (engine, action)
// references at load time. The action registry will implement it on
// Step 3; passing nil disables the check (default).
type ActionLookup interface {
	Has(engine, action string) bool
}

// Loader validates and loads pipeline YAML definitions.
//
// Zero value is usable: action-ref validation is off, schema validator
// is compiled lazily inside grammar.Compiled.
type Loader struct {
	// Actions, when non-nil, makes the loader reject step engine/action
	// pairs that the registry does not know about.
	Actions ActionLookup
}

// New returns a Loader with no action-ref validation.
func New() *Loader { return &Loader{} }

// LoadFile reads, validates, and returns one pipeline definition.
func (l *Loader) LoadFile(path string) (*Definition, error) {
	raw, err := readYAMLAsJSON(path)
	if err != nil {
		return nil, err
	}
	if err := l.validateSchema(raw, path); err != nil {
		return nil, err
	}
	pipeline, ok := raw["pipeline"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: %s: missing pipeline mapping", ErrLoad, filepath.Base(path))
	}

	if l.Actions != nil {
		if err := l.checkActionRefs(pipeline, path); err != nil {
			return nil, err
		}
	}
	requiresGraph, err := checkRequiresOrder(pipeline, path)
	if err != nil {
		return nil, err
	}
	if err := checkTemplating(pipeline, requiresGraph, path); err != nil {
		return nil, err
	}
	if err := checkTriggers(pipeline, path); err != nil {
		return nil, err
	}
	return buildDefinition(raw, pipeline, path)
}

// LoadDir loads every *.yaml file inside dir in sorted order.
//
// Missing dir → empty map (no error). Duplicate pipeline.name across
// files inside dir → ErrDuplicateName.
func (l *Loader) LoadDir(dir string) (map[string]*Definition, error) {
	out := map[string]*Definition{}
	owner := map[string]string{}

	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, fs.ErrNotExist) {
				return filepath.SkipAll
			}
			return walkErr
		}
		if path == dir {
			return nil
		}
		if d.IsDir() {
			return filepath.SkipDir
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".yaml") {
			return nil
		}
		defn, err := l.LoadFile(path)
		if err != nil {
			return err
		}
		if prev, dup := owner[defn.Name]; dup {
			return fmt.Errorf("%w: %q (in %s and %s)",
				ErrDuplicateName, defn.Name, filepath.Base(prev), filepath.Base(path))
		}
		owner[defn.Name] = path
		out[defn.Name] = defn
		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, fs.ErrNotExist) {
		return nil, walkErr
	}
	return out, nil
}

// LoadMany merges every dir in order. A pipeline.name found in two
// directories produces ErrDuplicateName — the first one wins in the
// error message, mirroring kernel.
func (l *Loader) LoadMany(dirs []string) (map[string]*Definition, error) {
	out := map[string]*Definition{}
	owner := map[string]string{}

	for _, dir := range dirs {
		local, err := l.LoadDir(dir)
		if err != nil {
			return nil, err
		}
		for name, defn := range local {
			if prev, dup := owner[name]; dup {
				return nil, fmt.Errorf("%w: %q (in %s and %s)",
					ErrDuplicateName, name, prev, defn.SourcePath)
			}
			owner[name] = defn.SourcePath
			out[name] = defn
		}
	}
	return out, nil
}

// --- internal validation steps --------------------------------------

// readYAMLAsJSON parses the file and converts it into the
// map[string]any shape expected by every downstream check.
//
// yaml.v3 decodes mapping keys into any-typed map[interface{}]interface{}
// which the jsonschema validator does not accept; we convert in one
// pass via JSON marshalling.
func readYAMLAsJSON(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: read %s: %v", ErrLoad, filepath.Base(path), err)
	}
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return nil, fmt.Errorf("%w: parse %s: %v", ErrLoad, filepath.Base(path), err)
	}
	// yaml.Node → any tree → JSON-round-trip → map[string]any
	var doc any
	if err := node.Decode(&doc); err != nil {
		return nil, fmt.Errorf("%w: decode %s: %v", ErrLoad, filepath.Base(path), err)
	}
	jsonBytes, err := json.Marshal(canonicalize(doc))
	if err != nil {
		return nil, fmt.Errorf("%w: canonical %s: %v", ErrLoad, filepath.Base(path), err)
	}
	var out map[string]any
	if err := json.Unmarshal(jsonBytes, &out); err != nil {
		return nil, fmt.Errorf("%w: %s: pipeline YAML root must be a mapping",
			ErrLoad, filepath.Base(path))
	}
	return out, nil
}

// canonicalize coerces any map[any]any introduced by yaml.v3 into
// map[string]any (rejecting non-string keys).
func canonicalize(v any) any {
	switch t := v.(type) {
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, child := range t {
			ks, ok := k.(string)
			if !ok {
				ks = fmt.Sprint(k)
			}
			out[ks] = canonicalize(child)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, child := range t {
			out[k] = canonicalize(child)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, child := range t {
			out[i] = canonicalize(child)
		}
		return out
	default:
		return v
	}
}

func (l *Loader) validateSchema(raw map[string]any, path string) error {
	v, err := grammar.Compiled()
	if err != nil {
		return fmt.Errorf("%w: compile grammar: %v", ErrLoad, err)
	}
	if err := v.Validate(raw); err != nil {
		return fmt.Errorf("%w: %s: %v", ErrSchema, filepath.Base(path), trimSchemaError(err))
	}
	return nil
}

// trimSchemaError converts a *jsonschema.ValidationError into its
// shortest human-readable message — full nested cause chain is too
// noisy for caller logs.
func trimSchemaError(err error) string {
	var ve *jsonschema.ValidationError
	if errors.As(err, &ve) {
		return ve.Message
	}
	return err.Error()
}

func (l *Loader) checkActionRefs(pipeline map[string]any, path string) error {
	steps, _ := pipeline["steps"].([]any)
	for _, raw := range steps {
		step, _ := raw.(map[string]any)
		eng, _ := step["engine"].(string)
		act, _ := step["action"].(string)
		if eng == "" || act == "" {
			continue
		}
		if !l.Actions.Has(eng, act) {
			return fmt.Errorf("%w: %s: step %q references (%s, %s)",
				ErrActionRef, filepath.Base(path), StepName(step), eng, act)
		}
	}
	return nil
}

// checkRequiresOrder also returns the requires graph as a
// step→[]dep map for downstream templating checks.
func checkRequiresOrder(pipeline map[string]any, path string) (map[string][]string, error) {
	steps, _ := pipeline["steps"].([]any)
	graph := make(map[string][]string, len(steps))
	seen := map[string]struct{}{}

	for _, raw := range steps {
		step, _ := raw.(map[string]any)
		name := StepName(step)
		deps := StepRequires(step)
		for _, dep := range deps {
			if _, ok := seen[dep]; !ok {
				return nil, fmt.Errorf("%w: %s: step %q requires %q which is not yet defined",
					ErrRequiresOrder, filepath.Base(path), name, dep)
			}
		}
		graph[name] = deps
		seen[name] = struct{}{}
	}
	return graph, nil
}

func checkTemplating(pipeline map[string]any, graph map[string][]string, path string) error {
	declared := map[string]struct{}{}
	if argsSchema, _ := pipeline["args"].(map[string]any); argsSchema != nil {
		if props, _ := argsSchema["properties"].(map[string]any); props != nil {
			for k := range props {
				declared[k] = struct{}{}
			}
		}
	}

	steps, _ := pipeline["steps"].([]any)
	for _, raw := range steps {
		step, _ := raw.(map[string]any)
		stepName := StepName(step)
		stepArgs, ok := step["args"].(map[string]any)
		if !ok {
			continue
		}
		reachable := transitiveRequires(stepName, graph)
		var firstErr error
		scanTemplateRefs(stepArgs, func(ref string) {
			if firstErr != nil {
				return
			}
			if strings.HasPrefix(ref, "args.") {
				key := strings.TrimPrefix(ref, "args.")
				if _, ok := declared[key]; !ok {
					firstErr = fmt.Errorf("%w: %s: step %q references ${args.%s} but %q is not declared in pipeline.args.properties",
						ErrTemplating, filepath.Base(path), stepName, key, key)
				}
				return
			}
			// steps.<sname>.result.<...>
			parts := strings.Split(ref, ".")
			if len(parts) < 2 {
				return
			}
			refStep := parts[1]
			if _, ok := reachable[refStep]; !ok {
				firstErr = fmt.Errorf("%w: %s: step %q references ${steps.%s.result.…} but %q is not in its transitive requires closure",
					ErrTemplating, filepath.Base(path), stepName, refStep, refStep)
			}
		})
		if firstErr != nil {
			return firstErr
		}
	}
	return nil
}

func checkTriggers(pipeline map[string]any, path string) error {
	triggers, _ := pipeline["triggers"].([]any)
	scheduleCount := 0
	declared := map[string]struct{}{}
	if argsSchema, _ := pipeline["args"].(map[string]any); argsSchema != nil {
		if props, _ := argsSchema["properties"].(map[string]any); props != nil {
			for k := range props {
				declared[k] = struct{}{}
			}
		}
	}

	for _, raw := range triggers {
		t, _ := raw.(map[string]any)
		switch t["type"] {
		case "schedule":
			scheduleCount++
			if scheduleCount > 1 {
				return fmt.Errorf("%w: %s: at most one schedule trigger allowed",
					ErrTrigger, filepath.Base(path))
			}
			if targs, _ := t["args"].(map[string]any); targs != nil {
				for k := range targs {
					if _, ok := declared[k]; !ok {
						return fmt.Errorf("%w: %s: schedule trigger arg %q not declared in pipeline.args.properties",
							ErrTrigger, filepath.Base(path), k)
					}
				}
			}
		case "mq":
			afp, _ := t["args_from_payload"].(map[string]any)
			for k, v := range afp {
				if !argNameRe.MatchString(k) {
					return fmt.Errorf("%w: %s: mq trigger args_from_payload key %q is not a valid arg name",
						ErrTrigger, filepath.Base(path), k)
				}
				s, ok := v.(string)
				if !ok || strings.TrimSpace(s) == "" {
					return fmt.Errorf("%w: %s: mq trigger args_from_payload value for %q must be a non-empty string",
						ErrTrigger, filepath.Base(path), k)
				}
			}
		}
	}
	return nil
}

func buildDefinition(raw, pipeline map[string]any, path string) (*Definition, error) {
	canonical, err := canonicalJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: canonical %s: %v", ErrLoad, filepath.Base(path), err)
	}
	sum := sha256.Sum256(canonical)
	hash := hex.EncodeToString(sum[:])

	out := &Definition{
		Name:          stringValue(pipeline["name"]),
		Version:       intValue(pipeline["version"]),
		SchemaVersion: intValue(pipeline["schema_version"]),
		SourcePath:    path,
		ContentHash:   hash,
		ArgsSchema:    asMap(pipeline["args"]),
		Triggers:      asMaps(pipeline["triggers"]),
		Steps:         asMaps(pipeline["steps"]),
		Raw:           raw,
	}
	return out, nil
}

// --- small JSON conversion helpers ----------------------------------

func canonicalJSON(v any) ([]byte, error) {
	// sort keys → deterministic content_hash regardless of YAML key
	// order
	return json.Marshal(sortMap(v))
}

func sortMap(v any) any {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make([]kv, 0, len(keys))
		for _, k := range keys {
			out = append(out, kv{K: k, V: sortMap(t[k])})
		}
		return sortedMap(out)
	case []any:
		out := make([]any, len(t))
		for i, child := range t {
			out[i] = sortMap(child)
		}
		return out
	default:
		return v
	}
}

type kv struct {
	K string
	V any
}

// sortedMap is a slice with stable key order that JSON-marshals as an
// object — go encoding/json sorts map keys, but it does so by string
// value of the type used as key (map[string]any), so we lean on the
// stdlib behaviour here.
type sortedMap []kv

func (s sortedMap) MarshalJSON() ([]byte, error) {
	out := make(map[string]any, len(s))
	for _, e := range s {
		out[e.K] = e.V
	}
	return json.Marshal(out)
}

func stringValue(v any) string {
	s, _ := v.(string)
	return s
}

func intValue(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	}
	return 0
}

func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

func asMaps(v any) []map[string]any {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(arr))
	for _, item := range arr {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}
