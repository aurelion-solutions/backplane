// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package cartridges

import (
	"errors"
	"net/http"
	"path/filepath"

	"github.com/labstack/echo/v4"
)

// CartridgeListItem is one entry in GET /cartridges.
type CartridgeListItem struct {
	ID            string `json:"id"`
	Version       string `json:"version,omitempty"`
	PipelineCount int    `json:"pipeline_count"`
	PolicyCount   int    `json:"policy_count"`
}

// CartridgeDetail is the response of GET /cartridges/{id}.
type CartridgeDetail struct {
	ID            string `json:"id"`
	Version       string `json:"version,omitempty"`
	Root          string `json:"root"`
	PipelineCount int    `json:"pipeline_count"`
	PolicyCount   int    `json:"policy_count"`
}

// PipelineFileItem is one entry in GET /cartridges/{id}/pipelines.
type PipelineFileItem struct {
	File string `json:"file"`
	Path string `json:"path"`
}

// AppListItem is one entry in GET /cartridges/{id}/apps.
type AppListItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version,omitempty"`
	Connector   string `json:"connector"`
	StatesCount int    `json:"states_count"`
	FieldsCount int    `json:"fields_count"`
}

// RegisterRoutes mounts the cartridges read-only HTTP surface on g.
func RegisterRoutes(g *echo.Group, provider Provider) {
	g.GET("/cartridges", listHandler(provider))
	g.GET("/cartridges/:id", detailHandler(provider))
	g.GET("/cartridges/:id/policies", listPoliciesHandler(provider))
	g.GET("/cartridges/:id/pipelines", listPipelinesHandler(provider))
	g.GET("/cartridges/:id/apps", listAppsHandler(provider))
	g.GET("/cartridges/:id/apps/:app_id", appDetailHandler(provider))
}

func listHandler(p Provider) echo.HandlerFunc {
	return func(c echo.Context) error {
		refs, err := p.List()
		if err != nil {
			return c.JSON(http.StatusInternalServerError, errorBody(err))
		}
		out := make([]CartridgeListItem, 0, len(refs))
		for _, ref := range refs {
			pipes, _ := p.Pipelines(ref)
			pols, _ := p.Policies(ref)
			out = append(out, CartridgeListItem{
				ID:            ref.ID,
				Version:       ref.Version,
				PipelineCount: len(pipes),
				PolicyCount:   len(pols),
			})
		}
		return c.JSON(http.StatusOK, out)
	}
}

func detailHandler(p Provider) echo.HandlerFunc {
	return func(c echo.Context) error {
		ref := Ref{ID: c.Param("id")}
		root, err := p.Materialize(ref)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return c.JSON(http.StatusNotFound, errorBody(err))
			}
			return c.JSON(http.StatusInternalServerError, errorBody(err))
		}
		pipes, _ := p.Pipelines(ref)
		pols, _ := p.Policies(ref)
		return c.JSON(http.StatusOK, CartridgeDetail{
			ID:            ref.ID,
			Version:       ref.Version,
			Root:          root,
			PipelineCount: len(pipes),
			PolicyCount:   len(pols),
		})
	}
}

func listPoliciesHandler(p Provider) echo.HandlerFunc {
	return func(c echo.Context) error {
		ref := Ref{ID: c.Param("id")}
		pols, err := p.Policies(ref)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return c.JSON(http.StatusNotFound, errorBody(err))
			}
			return c.JSON(http.StatusInternalServerError, errorBody(err))
		}
		out := make([]Manifest, 0, len(pols))
		for _, m := range pols {
			out = append(out, m)
		}
		return c.JSON(http.StatusOK, out)
	}
}

func listPipelinesHandler(p Provider) echo.HandlerFunc {
	return func(c echo.Context) error {
		ref := Ref{ID: c.Param("id")}
		paths, err := p.Pipelines(ref)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return c.JSON(http.StatusNotFound, errorBody(err))
			}
			return c.JSON(http.StatusInternalServerError, errorBody(err))
		}
		out := make([]PipelineFileItem, 0, len(paths))
		for _, p := range paths {
			out = append(out, PipelineFileItem{
				File: filepath.Base(p),
				Path: p,
			})
		}
		return c.JSON(http.StatusOK, out)
	}
}

func listAppsHandler(p Provider) echo.HandlerFunc {
	return func(c echo.Context) error {
		ref := Ref{ID: c.Param("id")}
		apps, err := p.Apps(ref)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return c.JSON(http.StatusNotFound, errorBody(err))
			}
			return c.JSON(http.StatusInternalServerError, errorBody(err))
		}
		out := make([]AppListItem, 0, len(apps))
		for _, a := range apps {
			out = append(out, AppListItem{
				ID:          a.Manifest.ID,
				Name:        a.Manifest.Name,
				Version:     a.Manifest.Version,
				Connector:   a.Manifest.Connector,
				StatesCount: len(a.Account.States),
				FieldsCount: len(a.Descriptor.Fields),
			})
		}
		return c.JSON(http.StatusOK, out)
	}
}

func appDetailHandler(p Provider) echo.HandlerFunc {
	return func(c echo.Context) error {
		ref := Ref{ID: c.Param("id")}
		appID := c.Param("app_id")
		apps, err := p.Apps(ref)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return c.JSON(http.StatusNotFound, errorBody(err))
			}
			return c.JSON(http.StatusInternalServerError, errorBody(err))
		}
		app, ok := apps[appID]
		if !ok {
			return c.JSON(http.StatusNotFound, errorBody(errors.New("app cartridge not found")))
		}
		return c.JSON(http.StatusOK, app)
	}
}

func errorBody(err error) map[string]string {
	return map[string]string{"detail": err.Error()}
}
