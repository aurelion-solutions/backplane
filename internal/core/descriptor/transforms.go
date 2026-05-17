// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package descriptor

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Transform is a post-template-execution string→string operation
// applied in pipeline order to a field's rendered value.
type Transform func(in string) (string, error)

// compileTransforms parses a YAML transforms list and returns the
// compiled pipeline. Each entry is either a plain name ("lower") or a
// parametrised form ("name:arg") split on the first colon. Unknown
// transforms and malformed parameters are rejected here so render
// time stays purely about data.
func compileTransforms(names []string) ([]Transform, error) {
	out := make([]Transform, 0, len(names))
	for _, raw := range names {
		name, arg, _ := strings.Cut(raw, ":")
		switch name {
		case "lower":
			out = append(out, transformLower)
		case "upper":
			out = append(out, transformUpper)
		case "remove_diacritics":
			out = append(out, transformRemoveDiacritics)
		case "truncate":
			n, err := strconv.Atoi(arg)
			if err != nil || n < 0 {
				return nil, fmt.Errorf("invalid truncate argument %q", arg)
			}
			out = append(out, makeTransformTruncate(n))
		default:
			return nil, fmt.Errorf("unknown transform %q", raw)
		}
	}
	return out, nil
}

func transformLower(in string) (string, error) { return strings.ToLower(in), nil }
func transformUpper(in string) (string, error) { return strings.ToUpper(in), nil }

// transformRemoveDiacritics normalises the string to NFD and drops
// every combining mark — "Iván" → "Ivan", "Müller" → "Muller".
func transformRemoveDiacritics(in string) (string, error) {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	out, _, err := transform.String(t, in)
	if err != nil {
		return "", err
	}
	return out, nil
}

// makeTransformTruncate clips the string to at most n runes. Counting
// is over runes, not bytes, so multibyte sequences are never split.
func makeTransformTruncate(n int) Transform {
	return func(in string) (string, error) {
		r := []rune(in)
		if len(r) <= n {
			return in, nil
		}
		return string(r[:n]), nil
	}
}
