// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package runner

import (
	"fmt"
	"regexp"
	"strings"
)

// templateRe matches ${args.X} and ${steps.<sname>.result.<path>} —
// re-declared from loader/templating.go so this package stays
// independent of the load-time validator.
var templateRe = regexp.MustCompile(
	`\$\{(args\.[a-zA-Z0-9_]+|steps\.[a-z][a-z0-9_]*\.result\.[a-zA-Z0-9_.]+)\}`,
)

// resolveTemplates walks v recursively and replaces every ${...}
// reference with the value from pipelineArgs (for ${args.X}) or
// stepResults (for ${steps.S.result.X.Y}). Pure single-reference
// strings return the resolved value as-is so int/list/map types are
// preserved; mixed strings stringify each match.
//
// Returns the first unresolved reference as an error.
func resolveTemplates(
	v any,
	pipelineArgs map[string]any,
	stepResults map[string]map[string]any,
) (any, error) {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, child := range t {
			r, err := resolveTemplates(child, pipelineArgs, stepResults)
			if err != nil {
				return nil, err
			}
			out[k] = r
		}
		return out, nil
	case []any:
		out := make([]any, len(t))
		for i, child := range t {
			r, err := resolveTemplates(child, pipelineArgs, stepResults)
			if err != nil {
				return nil, err
			}
			out[i] = r
		}
		return out, nil
	case string:
		// Pure-single-reference fast path preserves native type.
		if m := templateRe.FindStringSubmatchIndex(t); m != nil && m[0] == 0 && m[1] == len(t) {
			ref := t[m[2]:m[3]]
			return resolveRef(ref, pipelineArgs, stepResults)
		}
		// Mixed string: substitute each match.
		var resolveErr error
		out := templateRe.ReplaceAllStringFunc(t, func(match string) string {
			if resolveErr != nil {
				return match
			}
			ref := match[2 : len(match)-1]
			val, err := resolveRef(ref, pipelineArgs, stepResults)
			if err != nil {
				resolveErr = err
				return match
			}
			return fmt.Sprint(val)
		})
		return out, resolveErr
	default:
		return v, nil
	}
}

func resolveRef(ref string, pipelineArgs map[string]any, stepResults map[string]map[string]any) (any, error) {
	if strings.HasPrefix(ref, "args.") {
		key := strings.TrimPrefix(ref, "args.")
		val, ok := pipelineArgs[key]
		if !ok {
			return nil, fmt.Errorf("runner: ${args.%s} not provided in pipeline args", key)
		}
		return val, nil
	}
	parts := strings.Split(ref, ".")
	if len(parts) < 4 || parts[0] != "steps" || parts[2] != "result" {
		return nil, fmt.Errorf("runner: malformed template ref %q", ref)
	}
	stepName := parts[1]
	stepResult, ok := stepResults[stepName]
	if !ok {
		return nil, fmt.Errorf("runner: ${steps.%s.result.…} — step %q has no recorded result", stepName, stepName)
	}
	var node any = stepResult
	for _, segment := range parts[3:] {
		m, ok := node.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("runner: template ref %q walks into non-object at %q", ref, segment)
		}
		val, ok := m[segment]
		if !ok {
			return nil, fmt.Errorf("runner: template ref %q misses key %q", ref, segment)
		}
		node = val
	}
	return node, nil
}
