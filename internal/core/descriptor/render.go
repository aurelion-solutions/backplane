// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package descriptor

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"sort"
	"text/template"

	"github.com/aurelion-solutions/backplane/internal/core/cartridges"
)

// Inputs are the values bound to descriptor templates at render time.
//
// Principal is exposed as `.Principal`. Application is exposed as
// `.Application` and is typically the cartridges.AppManifest.Config
// map. TargetState selects the `by_state` branch for fields that
// have one.
type Inputs struct {
	Principal   any
	Application map[string]any
	TargetState string
}

// Result is the rendered descriptor: field name → resolved value.
//
// A field whose recipe is `by_state` and has no entry for the target
// state is omitted from the map entirely (rather than being set to
// nil).
type Result struct {
	Fields map[string]any
}

// Renderer compiles an AppCartridge's descriptor recipe once and
// renders it for as many (principal, target state) pairs as needed.
// A *Renderer is safe for concurrent use after construction.
type Renderer struct {
	order  []string
	fields map[string]*compiledField
}

// compiledField is one field of the recipe ready for execution.
// Exactly one of template / byState is set; the other is the zero
// value.
type compiledField struct {
	template    *template.Template
	byState     map[string]renderable
	transforms  []Transform
	onCollision Resolver
}

// renderable is the compiled form of one value: either a parsed
// template (when the YAML value was a string) or a raw scalar (when
// the YAML value was a number / bool — kept verbatim so a field like
// `userAccountControl.by_state.active: 512` lands in the descriptor
// as `int(512)`, not `"512"`).
type renderable struct {
	tmpl *template.Template
	raw  any
}

// descriptorRef matches `.Descriptor.<fieldname>` inside a template
// body. Used at compile time to extract cross-field dependencies for
// the topological sort. Field names follow Go identifier rules
// (letters, digits, underscore) which matches how text/template walks
// dot-paths.
var descriptorRef = regexp.MustCompile(`\.Descriptor\.([A-Za-z_][A-Za-z0-9_]*)`)

// NewRenderer compiles every field of the descriptor recipe and
// orders them so cross-field references render in the right order.
//
// resolvers may be nil — fields with on_collision then fall back to
// StubResolver. Returned errors cover: bad template syntax, unknown
// transform name, malformed transform parameter, reference to an
// undeclared descriptor field, and dependency cycles between fields.
func NewRenderer(app cartridges.AppCartridge, resolvers ResolverRegistry) (*Renderer, error) {
	fields := make(map[string]*compiledField, len(app.Descriptor.Fields))
	deps := make(map[string][]string, len(app.Descriptor.Fields))
	funcs := templateFuncs()

	for name, f := range app.Descriptor.Fields {
		cf, refs, err := compileField(name, f, funcs, resolvers)
		if err != nil {
			return nil, err
		}
		fields[name] = cf
		deps[name] = refs
	}

	order, err := topoSort(fields, deps)
	if err != nil {
		return nil, err
	}
	return &Renderer{order: order, fields: fields}, nil
}

// compileField turns one cartridges.DescriptorField into a
// compiledField, returning the set of `.Descriptor.X` references it
// makes (deduplicated, no particular order).
func compileField(
	name string,
	f cartridges.DescriptorField,
	funcs template.FuncMap,
	resolvers ResolverRegistry,
) (*compiledField, []string, error) {
	cf := &compiledField{}
	var refs []string

	switch {
	case f.Template != "":
		t, err := parseTemplate(name, funcs, f.Template)
		if err != nil {
			return nil, nil, fmt.Errorf("field %q: %w", name, err)
		}
		cf.template = t
		refs = extractDescriptorRefs(f.Template)

	case len(f.ByState) > 0:
		cf.byState = make(map[string]renderable, len(f.ByState))
		seen := map[string]struct{}{}
		for state, raw := range f.ByState {
			ren, r, err := compileRenderable(name+"["+state+"]", funcs, raw)
			if err != nil {
				return nil, nil, fmt.Errorf("field %q by_state[%s]: %w", name, state, err)
			}
			cf.byState[state] = ren
			for _, ref := range r {
				if _, ok := seen[ref]; ok {
					continue
				}
				seen[ref] = struct{}{}
				refs = append(refs, ref)
			}
		}

	default:
		// Defensive — validateAppCartridge already rejects this case
		// at load time; we return a typed error rather than panicking.
		return nil, nil, fmt.Errorf("field %q: neither template nor by_state set", name)
	}

	transforms, err := compileTransforms(f.Transforms)
	if err != nil {
		return nil, nil, fmt.Errorf("field %q transforms: %w", name, err)
	}
	cf.transforms = transforms

	if f.OnCollision != "" {
		if r, ok := resolvers[f.OnCollision]; ok {
			cf.onCollision = r
		} else {
			cf.onCollision = StubResolver
		}
	}
	return cf, refs, nil
}

// parseTemplate builds a strict template (missingkey=error) so a
// reference to an undefined `.Descriptor.X` fails loudly instead of
// silently rendering "<no value>".
func parseTemplate(name string, funcs template.FuncMap, body string) (*template.Template, error) {
	t, err := template.New(name).Option("missingkey=error").Funcs(funcs).Parse(body)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	return t, nil
}

// compileRenderable turns one YAML value into a renderable. Strings
// become parsed templates and contribute their `.Descriptor.X` refs;
// other scalars are kept verbatim with no refs.
func compileRenderable(name string, funcs template.FuncMap, raw any) (renderable, []string, error) {
	switch v := raw.(type) {
	case string:
		t, err := parseTemplate(name, funcs, v)
		if err != nil {
			return renderable{}, nil, err
		}
		return renderable{tmpl: t}, extractDescriptorRefs(v), nil
	default:
		return renderable{raw: v}, nil, nil
	}
}

// extractDescriptorRefs returns the deduplicated set of field names
// referenced via `.Descriptor.<name>` inside s.
func extractDescriptorRefs(s string) []string {
	matches := descriptorRef.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(matches))
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if _, ok := seen[m[1]]; ok {
			continue
		}
		seen[m[1]] = struct{}{}
		out = append(out, m[1])
	}
	return out
}

// topoSort returns a deterministic linear order in which every field
// is rendered after the fields it references. Determinism is
// important: error messages and test fixtures stay stable across
// runs. Returns an error when a field references an undeclared name
// or when the dependency graph contains a cycle.
func topoSort(fields map[string]*compiledField, deps map[string][]string) ([]string, error) {
	indegree := make(map[string]int, len(fields))
	successors := make(map[string][]string)

	for n := range fields {
		indegree[n] = 0
	}
	for n, refs := range deps {
		for _, ref := range refs {
			if _, ok := fields[ref]; !ok {
				return nil, fmt.Errorf("field %q references undeclared descriptor field %q", n, ref)
			}
			indegree[n]++
			successors[ref] = append(successors[ref], n)
		}
	}

	var ready []string
	for n, d := range indegree {
		if d == 0 {
			ready = append(ready, n)
		}
	}
	sort.Strings(ready)

	order := make([]string, 0, len(fields))
	for len(ready) > 0 {
		n := ready[0]
		ready = ready[1:]
		order = append(order, n)
		var next []string
		for _, m := range successors[n] {
			indegree[m]--
			if indegree[m] == 0 {
				next = append(next, m)
			}
		}
		sort.Strings(next)
		ready = append(ready, next...)
	}
	if len(order) != len(fields) {
		return nil, fmt.Errorf("cycle in descriptor field dependencies")
	}
	return order, nil
}

// Render executes the compiled recipe for one (principal, target
// state) pair.
//
// Field-level by_state recipes whose target state has no entry are
// omitted from the result. Template execution errors, transform
// errors and resolver errors all bubble up wrapped with the field
// name.
func (r *Renderer) Render(ctx context.Context, in Inputs) (Result, error) {
	rendered := make(map[string]any, len(r.fields))
	bindings := map[string]any{
		"Principal":   in.Principal,
		"Application": in.Application,
		"Descriptor":  rendered,
	}

	for _, name := range r.order {
		f := r.fields[name]
		value, ok, err := renderOne(ctx, name, f, in, bindings)
		if err != nil {
			return Result{}, err
		}
		if ok {
			rendered[name] = value
		}
	}
	return Result{Fields: rendered}, nil
}

// renderOne resolves one compiled field, returning (value, true,
// nil) when the field produced a value, (nil, false, nil) when a
// by_state recipe had no entry for the target state, or an error.
func renderOne(
	ctx context.Context,
	name string,
	f *compiledField,
	in Inputs,
	bindings map[string]any,
) (any, bool, error) {
	if f.template != nil {
		s, err := executeTemplate(f.template, bindings)
		if err != nil {
			return nil, false, fmt.Errorf("field %q: %w", name, err)
		}
		return applyPipeline(ctx, name, f, in, s)
	}

	ren, ok := f.byState[in.TargetState]
	if !ok {
		return nil, false, nil
	}
	if ren.tmpl == nil {
		// Raw scalar — transforms / collision do not apply to non-string values.
		return ren.raw, true, nil
	}
	s, err := executeTemplate(ren.tmpl, bindings)
	if err != nil {
		return nil, false, fmt.Errorf("field %q by_state[%s]: %w", name, in.TargetState, err)
	}
	return applyPipeline(ctx, name, f, in, s)
}

func executeTemplate(t *template.Template, bindings map[string]any) (string, error) {
	var buf bytes.Buffer
	if err := t.Execute(&buf, bindings); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// applyPipeline runs transforms and the on_collision resolver over
// the rendered string. Returns the final value plus ok=true.
func applyPipeline(
	ctx context.Context,
	name string,
	f *compiledField,
	in Inputs,
	s string,
) (any, bool, error) {
	for _, t := range f.transforms {
		var err error
		s, err = t(s)
		if err != nil {
			return nil, false, fmt.Errorf("field %q transform: %w", name, err)
		}
	}
	if f.onCollision != nil {
		res, err := f.onCollision.Resolve(ctx, ResolverInput{
			Field:       name,
			Value:       s,
			Principal:   in.Principal,
			Application: in.Application,
		})
		if err != nil {
			return nil, false, fmt.Errorf("field %q resolver: %w", name, err)
		}
		s = res
	}
	return s, true, nil
}
