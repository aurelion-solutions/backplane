// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package inventory_ingest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

// canonicalHashHex computes the sha256 of the canonical JSON
// representation of v and returns the hex-encoded digest.
//
// Canonical form: object keys sorted lexicographically, no
// insignificant whitespace, numbers and strings rendered as
// encoding/json would. Arrays preserve their order.
//
// Used to fingerprint a record so a re-delivery of the same content
// produces the same hash regardless of original map iteration order.
func canonicalHashHex(v any) (string, error) {
	var buf bytes.Buffer
	if err := writeCanonical(&buf, v); err != nil {
		return "", err
	}
	sum := sha256.Sum256(buf.Bytes())
	return hex.EncodeToString(sum[:]), nil
}

func writeCanonical(buf *bytes.Buffer, v any) error {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			kb, err := json.Marshal(k)
			if err != nil {
				return fmt.Errorf("canonical: marshal key: %w", err)
			}
			buf.Write(kb)
			buf.WriteByte(':')
			if err := writeCanonical(buf, x[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
		return nil
	case []any:
		buf.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeCanonical(buf, e); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
		return nil
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("canonical: marshal scalar: %w", err)
		}
		buf.Write(b)
		return nil
	}
}
