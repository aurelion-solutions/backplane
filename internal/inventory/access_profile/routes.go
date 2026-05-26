// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package access_profile

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// RegisterRoutes mounts the read-only access-profile endpoint.
func RegisterRoutes(g *echo.Group, svc *Service) {
	g.GET("/persons/:id/access-profile", profileHandler(svc))
}

// profileHandler returns the assembled access profile for one person.
// SAFE (read-only): never writes — fine on prefetch/HEAD.
func profileHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		idStr := c.Param("id")
		id, err := uuid.Parse(idStr)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"detail": "invalid person id: " + idStr})
		}
		profile, err := svc.Load(c.Request().Context(), id)
		if err != nil {
			if errors.Is(err, ErrPersonNotFound) {
				return c.JSON(http.StatusNotFound, map[string]string{"detail": "person not found"})
			}
			return c.JSON(http.StatusInternalServerError, map[string]string{"detail": err.Error()})
		}
		return c.JSON(http.StatusOK, profile)
	}
}
