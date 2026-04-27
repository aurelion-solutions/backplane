// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/marcboeker/go-duckdb/v2"
)

// AntiJoin reads every JSONL batch file under <base>/<datasetType>/,
// projects the latest revision per external_id by
// meta.committed_at DESC, and classifies each Candidate as new (no
// row in lake), changed (hash differs), or unchanged (omitted from
// the result).
//
// Implementation: in-memory DuckDB, read_json_auto() over the glob
// pattern, VALUES-CTE for the candidate set, LEFT JOIN for the
// classification. Cost is one DuckDB session + one scan per call.
// For batched ingest (windowed consumer) this runs once per
// (source, dataset_type) window — amortised cheaply.
func (s *File) AntiJoin(ctx context.Context, datasetType string, candidates []Candidate) (AntiJoinResult, error) {
	if err := validateDatasetType(datasetType); err != nil {
		return AntiJoinResult{}, err
	}
	if len(candidates) == 0 {
		return AntiJoinResult{}, nil
	}

	dir := filepath.Join(s.base, datasetType)
	if !hasJSONLFiles(dir) {
		// Dataset has no history yet — every candidate is new.
		out := AntiJoinResult{NewIDs: make([]string, len(candidates))}
		for i, c := range candidates {
			out.NewIDs[i] = c.ExternalID
		}
		return out, nil
	}

	db, err := sql.Open("duckdb", "")
	if err != nil {
		return AntiJoinResult{}, fmt.Errorf("storage/file: open duckdb: %w", err)
	}
	defer db.Close()

	glob := filepath.Join(dir, "*.jsonl")
	// Build a VALUES list for the candidate set. One bound parameter
	// per cell — external_id and hash never get string-formatted
	// into the SQL.
	var sb strings.Builder
	sb.WriteString(`
WITH incoming(external_id, hash) AS (VALUES `)
	args := make([]any, 0, len(candidates)*2)
	for i, c := range candidates {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString("(?, ?)")
		args = append(args, c.ExternalID, c.Hash)
	}
	sb.WriteString(`),
latest AS (
  SELECT external_id, hash
  FROM (
    SELECT
      external_id,
      meta.hash AS hash,
      ROW_NUMBER() OVER (
        PARTITION BY external_id
        ORDER BY meta.committed_at DESC
      ) AS rn
    FROM read_json_auto(?)
  )
  WHERE rn = 1
)
SELECT i.external_id,
       CASE WHEN l.external_id IS NULL THEN 'new' ELSE 'changed' END AS bucket
FROM incoming i
LEFT JOIN latest l USING (external_id)
WHERE l.hash IS NULL OR l.hash <> i.hash
`)
	args = append(args, glob)

	rows, err := db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return AntiJoinResult{}, fmt.Errorf("storage/file: duckdb anti-join: %w", err)
	}
	defer rows.Close()

	var out AntiJoinResult
	for rows.Next() {
		var id, bucket string
		if err := rows.Scan(&id, &bucket); err != nil {
			return AntiJoinResult{}, fmt.Errorf("storage/file: scan: %w", err)
		}
		switch bucket {
		case "new":
			out.NewIDs = append(out.NewIDs, id)
		case "changed":
			out.ChangedIDs = append(out.ChangedIDs, id)
		}
	}
	if err := rows.Err(); err != nil {
		return AntiJoinResult{}, fmt.Errorf("storage/file: rows err: %w", err)
	}
	return out, nil
}

// hasJSONLFiles reports whether dir exists and contains at least one
// .jsonl file. Used to short-circuit AntiJoin when the dataset has no
// history (DuckDB's read_json_auto over an empty glob errors).
func hasJSONLFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
			return true
		}
	}
	return false
}
