// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package applications

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// MatchingProvider returns connector instances whose tag set satisfies
// an Application's required_connector_tags. Implemented by
// integrations/connectors via a thin adapter in the composition root so
// the applications package does not import connectors directly. The
// return type is intentionally any — the wire shape is owned by the
// connectors package and serialised by echo.Context.JSON unchanged.
type MatchingProvider interface {
	MatchingForTags(ctx context.Context, requiredTags []string, onlineOnly bool) (any, error)
}

// RegisterRoutes mounts the Application HTTP surface on g. The matcher
// parameter wires the cross-slice connectors lookup; pass nil only when
// the connectors slice is intentionally disabled.
func RegisterRoutes(g *echo.Group, svc *Service, matcher MatchingProvider) {
	g.GET("/applications", listHandler(svc))
	g.POST("/applications", createHandler(svc))
	g.PATCH("/applications/:id", patchHandler(svc))
	g.DELETE("/applications/:id", deleteHandler(svc))
	if matcher != nil {
		g.GET("/applications/:id/matching-connector-instances", matchingHandler(svc, matcher))
	}
}

func listHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		apps, err := svc.List(c.Request().Context())
		if err != nil {
			return c.JSON(http.StatusInternalServerError, errorBody(err))
		}
		return c.JSON(http.StatusOK, apps)
	}
}

func createHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		var in CreatePayload
		if err := c.Bind(&in); err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		app, err := svc.Create(c.Request().Context(), in)
		if err != nil {
			if errors.Is(err, ErrCodeAlreadyExists) {
				return c.JSON(http.StatusConflict, errorBody(err))
			}
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		return c.JSON(http.StatusCreated, app)
	}
}

func patchHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		var in PatchPayload
		if err := c.Bind(&in); err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		app, err := svc.Update(c.Request().Context(), id, in)
		if err != nil {
			switch {
			case errors.Is(err, ErrNotFound):
				return c.JSON(http.StatusNotFound, errorBody(err))
			case errors.Is(err, ErrCodeAlreadyExists):
				return c.JSON(http.StatusConflict, errorBody(err))
			default:
				return c.JSON(http.StatusBadRequest, errorBody(err))
			}
		}
		return c.JSON(http.StatusOK, app)
	}
}

func deleteHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		if err := svc.Delete(c.Request().Context(), id); err != nil {
			if errors.Is(err, ErrNotFound) {
				return c.JSON(http.StatusNotFound, errorBody(err))
			}
			return c.JSON(http.StatusInternalServerError, errorBody(err))
		}
		return c.NoContent(http.StatusNoContent)
	}
}

func matchingHandler(svc *Service, matcher MatchingProvider) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		app, err := svc.Get(c.Request().Context(), id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return c.JSON(http.StatusNotFound, errorBody(err))
			}
			return c.JSON(http.StatusInternalServerError, errorBody(err))
		}
		onlineOnly := c.QueryParam("online_only") != "false"
		instances, err := matcher.MatchingForTags(c.Request().Context(), app.RequiredConnectorTags, onlineOnly)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, errorBody(err))
		}
		return c.JSON(http.StatusOK, instances)
	}
}

func errorBody(err error) map[string]string {
	return map[string]string{"detail": err.Error()}
}
