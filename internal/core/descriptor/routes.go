// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package descriptor

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/aurelion-solutions/backplane/internal/core/cartridges"
)

// RenderRequest is the body of POST
// /cartridges/{id}/apps/{app_id}/descriptor.
//
// Principal is bound to `.Principal` in templates. Application
// overrides the cartridge manifest's `config:` block when set —
// otherwise the manifest defaults are used. TargetState selects the
// `by_state` branch and is mandatory.
type RenderRequest struct {
	Principal   map[string]any `json:"principal"`
	Application map[string]any `json:"application,omitempty"`
	TargetState string         `json:"target_state"`
}

// RenderResponse is the body of POST /…/descriptor.
type RenderResponse struct {
	Fields map[string]any `json:"fields"`
}

// RegisterRoutes mounts the descriptor render endpoint on g.
//
// Cyclic-dependency-wise the render endpoint cannot live in the
// cartridges package — descriptor already imports cartridges, so the
// HTTP surface that bridges them lives here.
func RegisterRoutes(g *echo.Group, provider cartridges.Provider) {
	g.POST("/cartridges/:id/apps/:app_id/descriptor", renderHandler(provider))
}

func renderHandler(p cartridges.Provider) echo.HandlerFunc {
	return func(c echo.Context) error {
		ref := cartridges.Ref{ID: c.Param("id")}
		appID := c.Param("app_id")

		apps, err := p.Apps(ref)
		if err != nil {
			if errors.Is(err, cartridges.ErrNotFound) {
				return c.JSON(http.StatusNotFound, errorBody(err))
			}
			return c.JSON(http.StatusInternalServerError, errorBody(err))
		}
		app, ok := apps[appID]
		if !ok {
			return c.JSON(http.StatusNotFound,
				errorBody(errors.New("app cartridge not found")))
		}

		var req RenderRequest
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		if req.TargetState == "" {
			return c.JSON(http.StatusBadRequest,
				errorBody(errors.New("target_state is required")))
		}

		r, err := NewRenderer(app, nil)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, errorBody(err))
		}
		result, err := r.Render(c.Request().Context(), Inputs{
			Principal:   req.Principal,
			Application: mergeApplication(app.Manifest.Config, req.Application),
			TargetState: req.TargetState,
		})
		if err != nil {
			return c.JSON(http.StatusUnprocessableEntity, errorBody(err))
		}
		return c.JSON(http.StatusOK, RenderResponse{Fields: result.Fields})
	}
}

// mergeApplication overlays the request's override onto the manifest
// config map without mutating either. Override wins per key. Returns
// the manifest map as-is when override is nil so callers that only
// want the cartridge defaults pay no copy.
func mergeApplication(manifest, override map[string]any) map[string]any {
	if len(override) == 0 {
		return manifest
	}
	out := make(map[string]any, len(manifest)+len(override))
	for k, v := range manifest {
		out[k] = v
	}
	for k, v := range override {
		out[k] = v
	}
	return out
}

func errorBody(err error) map[string]string {
	return map[string]string{"detail": err.Error()}
}
