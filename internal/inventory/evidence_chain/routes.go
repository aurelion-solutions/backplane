// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package evidence_chain

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// RegisterRoutes mounts the read-only evidence-chain surface on g.
// Chains are written by the worker policy-assessment action.
func RegisterRoutes(g *echo.Group, repo Repository) {
	g.GET("/evidence-chains/:id", getHandler(repo))
}

func getHandler(repo Repository) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"detail": err.Error()})
		}
		row, err := repo.GetByID(c.Request().Context(), id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return c.JSON(http.StatusNotFound, map[string]string{"detail": err.Error()})
			}
			return c.JSON(http.StatusInternalServerError, map[string]string{"detail": err.Error()})
		}
		return c.JSON(http.StatusOK, row)
	}
}
