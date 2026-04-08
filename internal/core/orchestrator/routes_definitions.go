// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package orchestrator

import (
	"net/http"
	"os"

	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/loader"
	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/registry"
	"github.com/labstack/echo/v4"
)

// PipelineSummary is one entry in GET /api/v0/pipelines.
//
// `triggers` is returned in full (the same shape detail uses) instead
// of a bare count so the engineering-studio renderer can decorate
// each list row with the trigger type / routing key without a second
// round-trip per pipeline. `description` is reserved — grammar does
// not declare it yet, so it is always null.
type PipelineSummary struct {
	Name          string           `json:"name"`
	Version       int              `json:"version"`
	SchemaVersion int              `json:"schema_version"`
	ContentHash   string           `json:"content_hash"`
	SourcePath    string           `json:"source_path"`
	StepCount     int              `json:"step_count"`
	Triggers      []map[string]any `json:"triggers"`
	Description   *string          `json:"description"`
}

// PipelineDetail is the response of GET /api/v0/pipelines/{name}.
type PipelineDetail struct {
	Name          string           `json:"name"`
	Version       int              `json:"version"`
	SchemaVersion int              `json:"schema_version"`
	ContentHash   string           `json:"content_hash"`
	SourcePath    string           `json:"source_path"`
	Args          map[string]any   `json:"args_schema"`
	Triggers      []map[string]any `json:"triggers"`
	Steps         []map[string]any `json:"steps"`
}

// RegisterDefinitionRoutes mounts the read-only definition / action
// catalogue endpoints on g. Mutating run endpoints (POST /runs, ...)
// land in Step 6.
func RegisterDefinitionRoutes(g *echo.Group, catalog *Catalog, reg *registry.Registry) {
	g.GET("/pipelines", listPipelinesHandler(catalog))
	g.GET("/pipelines/:name", getPipelineHandler(catalog))
	g.GET("/pipelines/:name/source", getPipelineSourceHandler(catalog))
	g.GET("/actions", listActionsHandler(reg))
}

// RegisterWellKnownRoutes mounts the .well-known schema endpoint at
// /.well-known/pipeline-schema.json (mounted on the root group, not
// /api/v0).
func RegisterWellKnownRoutes(g *echo.Echo, reg *registry.Registry) {
	g.GET("/.well-known/pipeline-schema.json", pipelineSchemaHandler(reg))
}

func listPipelinesHandler(c *Catalog) echo.HandlerFunc {
	return func(ec echo.Context) error {
		if c == nil {
			return ec.JSON(http.StatusOK, []PipelineSummary{})
		}
		out := make([]PipelineSummary, 0, len(c.All()))
		for _, d := range c.All() {
			out = append(out, summaryOf(d))
		}
		return ec.JSON(http.StatusOK, out)
	}
}

func getPipelineHandler(c *Catalog) echo.HandlerFunc {
	return func(ec echo.Context) error {
		if c == nil {
			return ec.JSON(http.StatusNotFound, map[string]string{"detail": "no catalog"})
		}
		name := ec.Param("name")
		d := c.Get(name)
		if d == nil {
			return ec.JSON(http.StatusNotFound, map[string]string{
				"detail": "pipeline " + name + " not loaded",
			})
		}
		return ec.JSON(http.StatusOK, detailOf(d))
	}
}

// getPipelineSourceHandler returns the raw YAML of a pipeline as
// `text/plain; charset=utf-8`. The path is taken from the in-memory
// Definition the loader stamped at startup, so it is trusted (not user
// input) — we read the file fresh on every request so an operator who
// edits the YAML on disk between cartridge reloads sees the current
// contents of the file the loader saw.
func getPipelineSourceHandler(c *Catalog) echo.HandlerFunc {
	return func(ec echo.Context) error {
		if c == nil {
			return ec.JSON(http.StatusNotFound, map[string]string{"detail": "no catalog"})
		}
		name := ec.Param("name")
		d := c.Get(name)
		if d == nil {
			return ec.JSON(http.StatusNotFound, map[string]string{
				"detail": "pipeline " + name + " not loaded",
			})
		}
		if d.SourcePath == "" {
			return ec.JSON(http.StatusNotFound, map[string]string{
				"detail": "pipeline " + name + " has no source path",
			})
		}
		raw, err := os.ReadFile(d.SourcePath)
		if err != nil {
			return ec.JSON(http.StatusInternalServerError, map[string]string{
				"detail": "read source: " + err.Error(),
			})
		}
		return ec.Blob(http.StatusOK, "text/plain; charset=utf-8", raw)
	}
}

func listActionsHandler(reg *registry.Registry) echo.HandlerFunc {
	return func(ec echo.Context) error {
		return ec.JSON(http.StatusOK, BuildActionCatalogue(reg))
	}
}

func pipelineSchemaHandler(reg *registry.Registry) echo.HandlerFunc {
	return func(ec echo.Context) error {
		schema, err := BuildMergedSchema(reg)
		if err != nil {
			return ec.JSON(http.StatusInternalServerError, map[string]string{"detail": err.Error()})
		}
		return ec.JSON(http.StatusOK, schema)
	}
}

func summaryOf(d *loader.Definition) PipelineSummary {
	return PipelineSummary{
		Name:          d.Name,
		Version:       d.Version,
		SchemaVersion: d.SchemaVersion,
		ContentHash:   d.ContentHash,
		SourcePath:    d.SourcePath,
		StepCount:     len(d.Steps),
		Triggers:      ensureMaps(d.Triggers),
		Description:   nil,
	}
}

// ensureMaps coerces a nil Triggers slice into an empty array so the
// wire shape is `"triggers": []` rather than `"triggers": null` — the
// engineering-studio renderer reads `.length` on the field directly.
func ensureMaps(in []map[string]any) []map[string]any {
	if in == nil {
		return []map[string]any{}
	}
	return in
}

func detailOf(d *loader.Definition) PipelineDetail {
	args := d.ArgsSchema
	if args == nil {
		args = map[string]any{}
	}
	return PipelineDetail{
		Name:          d.Name,
		Version:       d.Version,
		SchemaVersion: d.SchemaVersion,
		ContentHash:   d.ContentHash,
		SourcePath:    d.SourcePath,
		Args:          args,
		Triggers:      ensureMaps(d.Triggers),
		Steps:         ensureMaps(d.Steps),
	}
}
