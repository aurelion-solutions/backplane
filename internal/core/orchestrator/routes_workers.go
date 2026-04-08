// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package orchestrator

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/uptrace/bun"
)

// WorkerListResponse is the body of GET /api/v0/workers.
type WorkerListResponse struct {
	Items []WorkerSummary `json:"items"`
}

// RegisterWorkerRoutes mounts the read-only workers endpoints.
//
// /workers returns a derived per-slot view computed from
// pipeline_runs; there is no separate workers table.
func RegisterWorkerRoutes(g *echo.Group, db *bun.DB, svc *Service) {
	g.GET("/workers", listWorkersHandler(db, svc))
	g.GET("/workers/:worker_id/runs", listWorkerRunsHandler(db, svc))
}

func listWorkersHandler(db *bun.DB, svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		var items []WorkerSummary
		err := db.RunInTx(c.Request().Context(), nil, func(ctx context.Context, tx bun.Tx) error {
			var inner error
			items, inner = svc.ListWorkers(ctx, tx)
			return inner
		})
		if err != nil {
			return c.JSON(http.StatusInternalServerError, errBody(err.Error()))
		}
		if items == nil {
			items = []WorkerSummary{}
		}
		return c.JSON(http.StatusOK, WorkerListResponse{Items: items})
	}
}

func listWorkerRunsHandler(db *bun.DB, svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		workerID := c.Param("worker_id")
		if workerID == "" {
			return c.JSON(http.StatusBadRequest, errBody("missing worker_id"))
		}
		limit := parseInt(c.QueryParam("limit"), 50)
		offset := parseInt(c.QueryParam("offset"), 0)
		var items []*PipelineRun
		var total int
		err := db.RunInTx(c.Request().Context(), nil, func(ctx context.Context, tx bun.Tx) error {
			var inner error
			items, total, inner = svc.ListRunsByWorker(ctx, tx, workerID, limit, offset)
			return inner
		})
		if err != nil {
			return c.JSON(http.StatusInternalServerError, errBody(err.Error()))
		}
		return c.JSON(http.StatusOK, RunListResponse{Items: items, Total: total, Limit: limit, Offset: offset})
	}
}
