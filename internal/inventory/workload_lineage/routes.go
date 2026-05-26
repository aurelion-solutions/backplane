// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package workload_lineage

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// RegisterRoutes mounts the lineage read endpoint on g.
// Note: NO SnapshotWriter dependency — GET is read-only (R1).
// Snapshot writes happen exclusively in the assess pass (cmd/worker).
func RegisterRoutes(g *echo.Group, resolver *Resolver) {
	g.GET("/workloads/:id/lineage", lineageHandler(resolver))
}

// lineageHandler returns the resolved ownership chain for a workload.
//
// This handler is SAFE (read-only): it resolves and returns the chain
// but MUST NOT write a snapshot. A GET with a write side-effect would
// fire on prefetch / HEAD / cache revalidation and pollute the
// append-only ledger. Snapshot persistence lives in the assess action.
func lineageHandler(resolver *Resolver) echo.HandlerFunc {
	return func(c echo.Context) error {
		idStr := c.Param("id")
		id, err := uuid.Parse(idStr)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"detail": "invalid workload id: " + idStr})
		}

		chain, err := resolver.Resolve(c.Request().Context(), id)
		if err != nil {
			if errors.Is(err, ErrWorkloadNotFound) {
				return c.JSON(http.StatusNotFound, map[string]string{"detail": "workload not found"})
			}
			return c.JSON(http.StatusInternalServerError, map[string]string{"detail": err.Error()})
		}

		return c.JSON(http.StatusOK, chain)
	}
}
