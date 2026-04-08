// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package loader

import "regexp"

// templateRe matches ${args.X} and ${steps.<sname>.result.<path>} in
// string values nested anywhere inside step args. Mirrors the kernel
// loader regex verbatim.
var templateRe = regexp.MustCompile(
	`\$\{(args\.[a-zA-Z0-9_]+|steps\.[a-z][a-z0-9_]*\.result\.[a-zA-Z0-9_.]+)\}`,
)

// transitiveRequires walks the requires graph breadth-first and returns
// every ancestor of stepName.
func transitiveRequires(stepName string, graph map[string][]string) map[string]struct{} {
	out := map[string]struct{}{}
	queue := append([]string(nil), graph[stepName]...)
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		if _, seen := out[node]; seen {
			continue
		}
		out[node] = struct{}{}
		queue = append(queue, graph[node]...)
	}
	return out
}

// scanTemplateRefs invokes fn for every ${...} match found within v
// (recursing into nested maps and arrays).
//
// Pure value-walk — no I/O, no allocations beyond regex matches.
func scanTemplateRefs(v any, fn func(ref string)) {
	switch t := v.(type) {
	case string:
		for _, m := range templateRe.FindAllStringSubmatch(t, -1) {
			fn(m[1])
		}
	case map[string]any:
		for _, child := range t {
			scanTemplateRefs(child, fn)
		}
	case []any:
		for _, child := range t {
			scanTemplateRefs(child, fn)
		}
	}
}
