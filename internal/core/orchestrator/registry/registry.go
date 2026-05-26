// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package registry

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	invopop "github.com/invopop/jsonschema"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

// Handler is the function shape an action handler must satisfy.
//
// A and R are concrete struct types (or named map types). They MUST
// be JSON-tag-decorated so invopop produces a meaningful schema.
type Handler[A, R any] func(args A, ctx ActionContext) (R, error)

// Entry is the type-erased registered record. Stored under
// (engine, action) inside Registry.
type Entry struct {
	Engine       string
	Action       string
	Idempotent   bool
	ArgsSchema   map[string]any
	ResultSchema map[string]any

	argsValidator   *jsonschema.Schema
	resultValidator *jsonschema.Schema
	dispatch        dispatchFn
}

type dispatchFn func(raw map[string]any, ctx ActionContext) (map[string]any, error)

// Registry is an in-memory store keyed by (engine, action).
//
// Safe for concurrent reads after composition is finished. Concurrent
// Register calls are also safe, but in practice every engine
// registers at process startup before the runner starts dispatching.
type Registry struct {
	mu      sync.RWMutex
	entries map[[2]string]*Entry
}

// New returns an empty Registry.
func New() *Registry {
	return &Registry{entries: make(map[[2]string]*Entry)}
}

// Get returns the registered Entry for the pair, or nil + ErrNotFound.
func (r *Registry) Get(engine, action string) (*Entry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.entries[[2]string{engine, action}]
	if !ok {
		return nil, fmt.Errorf("%w: (%q, %q)", ErrNotFound, engine, action)
	}
	return e, nil
}

// Has implements loader.ActionLookup so the YAML loader can reject
// step refs to unregistered actions.
func (r *Registry) Has(engine, action string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.entries[[2]string{engine, action}]
	return ok
}

// All returns every registered Entry sorted by engine, then action.
func (r *Registry) All() []*Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Entry, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Engine != out[j].Engine {
			return out[i].Engine < out[j].Engine
		}
		return out[i].Action < out[j].Action
	})
	return out
}

// Dispatch runs the registered handler against raw args. It does:
//   - args JSON Schema validation
//   - args JSON → A unmarshalling
//   - handler invocation
//   - result JSON Schema validation
//   - result struct → map[string]any conversion
//
// On any failure the partial state is discarded — the caller (runner)
// is responsible for rolling back ctx.Tx and recording the failure.
func (r *Registry) Dispatch(engine, action string, raw map[string]any, ctx ActionContext) (map[string]any, error) {
	entry, err := r.Get(engine, action)
	if err != nil {
		return nil, err
	}
	return entry.dispatch(raw, ctx)
}

// Register hooks a generic handler into r under (engine, action).
//
// Calling Register twice with the same pair returns ErrDuplicate.
// idempotent is metadata used by the runner to decide retry policy.
func Register[A, R any](r *Registry, engine, action string, idempotent bool, h Handler[A, R]) error {
	if engine == "" || action == "" {
		return fmt.Errorf("registry: empty engine or action")
	}

	argsSchema, argsValidator, err := buildSchema[A]()
	if err != nil {
		return fmt.Errorf("%w: args of (%q, %q): %v", ErrSchemaGeneration, engine, action, err)
	}
	resultSchema, resultValidator, err := buildSchema[R]()
	if err != nil {
		return fmt.Errorf("%w: result of (%q, %q): %v", ErrSchemaGeneration, engine, action, err)
	}

	entry := &Entry{
		Engine:          engine,
		Action:          action,
		Idempotent:      idempotent,
		ArgsSchema:      argsSchema,
		ResultSchema:    resultSchema,
		argsValidator:   argsValidator,
		resultValidator: resultValidator,
		dispatch: func(raw map[string]any, ctx ActionContext) (map[string]any, error) {
			// 1. Validate raw against args schema.
			if err := argsValidator.Validate(raw); err != nil {
				return nil, fmt.Errorf("%w: %v", ErrArgsValidation, err)
			}
			// 2. JSON-roundtrip raw → A.
			var args A
			if err := decode(raw, &args); err != nil {
				return nil, fmt.Errorf("%w: decode args: %v", ErrArgsValidation, err)
			}
			// 3. Call handler.
			result, err := h(args, ctx)
			if err != nil {
				return nil, err
			}
			// 4. Encode result → map[string]any.
			out, err := encode(result)
			if err != nil {
				return nil, fmt.Errorf("registry: encode result: %v", err)
			}
			// 5. Validate result against result schema.
			if err := resultValidator.Validate(out); err != nil {
				return nil, fmt.Errorf("%w: %v", ErrResultValidation, err)
			}
			return out, nil
		},
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	key := [2]string{engine, action}
	if _, dup := r.entries[key]; dup {
		return fmt.Errorf("%w: (%q, %q)", ErrDuplicate, engine, action)
	}
	r.entries[key] = entry
	return nil
}

// MustRegister is the panic-on-error variant. Use in composition code
// where a registration failure means the binary cannot serve traffic
// safely.
func MustRegister[A, R any](r *Registry, engine, action string, idempotent bool, h Handler[A, R]) {
	if err := Register(r, engine, action, idempotent, h); err != nil {
		panic(err)
	}
}

// --- internal helpers -----------------------------------------------

// buildSchema reflects T, materialises the JSON Schema via invopop,
// and compiles it through santhosh-tekuri for runtime validation.
// The returned map[string]any is the marshalled view of the same
// schema (used by the well-known endpoint).
func buildSchema[T any]() (map[string]any, *jsonschema.Schema, error) {
	var zero T
	reflector := &invopop.Reflector{
		// Anonymous, lazy-decoded args structs typically have flat
		// JSON; we keep allow-additional-properties = false so unknown
		// keys at handler input are caught early.
		ExpandedStruct:             true,
		DoNotReference:             true,
		RequiredFromJSONSchemaTags: false,
	}
	rawSchema := reflector.Reflect(zero)
	if rawSchema == nil {
		return nil, nil, fmt.Errorf("invopop returned nil for %T", zero)
	}
	body, err := json.Marshal(rawSchema)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, nil, fmt.Errorf("unmarshal: %v", err)
	}
	c := jsonschema.NewCompiler()
	c.Draft = jsonschema.Draft2020
	const url = "mem://schema.json"
	if err := c.AddResource(url, bytes.NewReader(body)); err != nil {
		return nil, nil, fmt.Errorf("compile add resource: %v", err)
	}
	validator, err := c.Compile(url)
	if err != nil {
		return nil, nil, fmt.Errorf("compile: %v", err)
	}
	return m, validator, nil
}

// decode round-trips raw → T via JSON.
func decode(raw map[string]any, out any) error {
	body, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

// encode round-trips any → map[string]any via JSON.
func encode(v any) (map[string]any, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return out, nil
}
