// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package orchestrator

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/uptrace/bun"
)

// CreateRunRequest is the payload of POST /api/v0/pipelines/{name}/runs.
type CreateRunRequest struct {
	PipelineVersion *int           `json:"pipeline_version,omitempty"`
	Args            map[string]any `json:"args,omitempty"`
}

// CreateRunResponse is the body returned by POST.
type CreateRunResponse struct {
	RunID           uuid.UUID `json:"run_id"`
	PipelineName    string    `json:"pipeline_name"`
	PipelineVersion int       `json:"pipeline_version"`
	Status          RunStatus `json:"status"`
	Created         bool      `json:"created"`
}

// RunDetail is the body of GET /api/v0/pipelines/runs/{id}.
type RunDetail struct {
	Run   *PipelineRun `json:"run"`
	Steps []*StepRun   `json:"steps"`
}

// RunListResponse is the envelope of GET /api/v0/pipelines/runs and
// /api/v0/pipelines/{name}/runs.
type RunListResponse struct {
	Items  []*PipelineRun `json:"items"`
	Total  int            `json:"total"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
}

// ResolveStepRequest is the HITL body of
// POST /api/v0/pipelines/runs/{id}/steps/{step}/resolve.
type ResolveStepRequest struct {
	Payload map[string]any `json:"payload"`
}

// RetryRunResponse is the compact response body for
// POST /api/v0/pipelines/runs/{id}/retry.
type RetryRunResponse struct {
	RunID           uuid.UUID `json:"run_id"`
	RetryOfRunID    uuid.UUID `json:"retry_of_run_id"`
	Status          RunStatus `json:"status"`
	PipelineName    string    `json:"pipeline_name"`
	PipelineVersion int       `json:"pipeline_version"`
}

// RegisterRunRoutes mounts the mutating run endpoints on g.
func RegisterRunRoutes(g *echo.Group, db *bun.DB, svc *Service, catalog *Catalog) {
	g.POST("/pipelines/:name/runs", createRunHandler(db, svc, catalog))
	g.GET("/pipelines/:name/runs", listRunsByPipelineHandler(db, svc))
	g.GET("/pipelines/runs", listRunsHandler(db, svc))
	g.GET("/pipelines/runs/:id", getRunHandler(db, svc))
	g.GET("/pipelines/runs/:id/steps", listStepsHandler(db, svc))
	g.GET("/pipelines/runs/:id/steps/:step", getStepHandler(db, svc))
	g.POST("/pipelines/runs/:id/cancel", cancelRunHandler(db, svc))
	g.POST("/pipelines/runs/:id/retry", retryRunHandler(db, svc))
	g.POST("/pipelines/runs/:id/steps/:step/resolve", resolveStepHandler(db, svc))
}

func createRunHandler(db *bun.DB, svc *Service, catalog *Catalog) echo.HandlerFunc {
	return func(c echo.Context) error {
		name := c.Param("name")
		if name == "runs" {
			return c.JSON(http.StatusNotFound, errBody("pipeline 'runs' is reserved"))
		}
		def := catalog.Get(name)
		if def == nil {
			return c.JSON(http.StatusNotFound, errBody("pipeline "+name+" not loaded"))
		}
		var body CreateRunRequest
		if err := c.Bind(&body); err != nil {
			return c.JSON(http.StatusBadRequest, errBody(err.Error()))
		}
		version := def.Version
		if body.PipelineVersion != nil {
			if *body.PipelineVersion != def.Version {
				return c.JSON(http.StatusNotFound,
					errBody("pipeline version mismatch (current: "+strconv.Itoa(def.Version)+")"))
			}
			version = *body.PipelineVersion
		}

		var res CreateRunResult
		err := db.RunInTx(c.Request().Context(), nil, func(ctx context.Context, tx bun.Tx) error {
			var inner error
			res, inner = svc.CreateRun(ctx, tx, CreateRunInput{
				PipelineName:    name,
				PipelineVersion: version,
				Args:            body.Args,
				TriggerSource:   TriggerHTTP,
			})
			return inner
		})
		if err != nil {
			return c.JSON(http.StatusInternalServerError, errBody(err.Error()))
		}
		status := http.StatusCreated
		if !res.Created {
			status = http.StatusOK
		}
		return c.JSON(status, CreateRunResponse{
			RunID:           res.Run.ID,
			PipelineName:    res.Run.PipelineName,
			PipelineVersion: res.Run.PipelineVersion,
			Status:          res.Run.Status,
			Created:         res.Created,
		})
	}
}

func listRunsByPipelineHandler(db *bun.DB, svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		name := c.Param("name")
		if name == "runs" {
			return c.JSON(http.StatusNotFound, errBody("pipeline 'runs' is reserved"))
		}
		limit := parseInt(c.QueryParam("limit"), 50)
		offset := parseInt(c.QueryParam("offset"), 0)
		statuses := parseStatuses(c.QueryParams()["status"])
		filters := ListRunsFilters{Pipeline: name, Statuses: statuses, Limit: limit, Offset: offset}
		return listRunsExec(c, db, svc, filters, limit, offset)
	}
}

func listRunsHandler(db *bun.DB, svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		limit := parseInt(c.QueryParam("limit"), 50)
		offset := parseInt(c.QueryParam("offset"), 0)
		statuses := parseStatuses(c.QueryParams()["status"])
		filters := ListRunsFilters{
			Pipeline: c.QueryParam("pipeline"),
			Statuses: statuses,
			Limit:    limit,
			Offset:   offset,
		}
		return listRunsExec(c, db, svc, filters, limit, offset)
	}
}

func listRunsExec(c echo.Context, db *bun.DB, svc *Service, f ListRunsFilters, limit, offset int) error {
	var items []*PipelineRun
	var total int
	err := db.RunInTx(c.Request().Context(), nil, func(ctx context.Context, tx bun.Tx) error {
		var inner error
		items, total, inner = svc.ListRuns(ctx, tx, f)
		return inner
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errBody(err.Error()))
	}
	return c.JSON(http.StatusOK, RunListResponse{Items: items, Total: total, Limit: limit, Offset: offset})
}

func getRunHandler(db *bun.DB, svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, errBody("invalid run id"))
		}
		var run *PipelineRun
		var steps []*StepRun
		txErr := db.RunInTx(c.Request().Context(), nil, func(ctx context.Context, tx bun.Tx) error {
			r, err := svc.GetRun(ctx, tx, id)
			if err != nil {
				return err
			}
			run = r
			s, err := svc.ListStepsByRun(ctx, tx, id)
			if err != nil {
				return err
			}
			steps = s
			return nil
		})
		if txErr != nil {
			if errors.Is(txErr, ErrNotFound) {
				return c.JSON(http.StatusNotFound, errBody("run not found"))
			}
			return c.JSON(http.StatusInternalServerError, errBody(txErr.Error()))
		}
		return c.JSON(http.StatusOK, RunDetail{Run: run, Steps: steps})
	}
}

func listStepsHandler(db *bun.DB, svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, errBody("invalid run id"))
		}
		var steps []*StepRun
		txErr := db.RunInTx(c.Request().Context(), nil, func(ctx context.Context, tx bun.Tx) error {
			out, err := svc.ListStepsByRun(ctx, tx, id)
			steps = out
			return err
		})
		if txErr != nil {
			return c.JSON(http.StatusInternalServerError, errBody(txErr.Error()))
		}
		return c.JSON(http.StatusOK, steps)
	}
}

func getStepHandler(db *bun.DB, svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, errBody("invalid run id"))
		}
		stepName := c.Param("step")
		var step *StepRun
		txErr := db.RunInTx(c.Request().Context(), nil, func(ctx context.Context, tx bun.Tx) error {
			s, err := svc.LatestStepAttempt(ctx, tx, id, stepName)
			step = s
			return err
		})
		if txErr != nil {
			if errors.Is(txErr, ErrNotFound) {
				return c.JSON(http.StatusNotFound, errBody("step not found"))
			}
			return c.JSON(http.StatusInternalServerError, errBody(txErr.Error()))
		}
		return c.JSON(http.StatusOK, step)
	}
}

func cancelRunHandler(db *bun.DB, svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, errBody("invalid run id"))
		}
		var outcome CancelOutcome
		txErr := db.RunInTx(c.Request().Context(), nil, func(ctx context.Context, tx bun.Tx) error {
			out, err := svc.RequestCancel(ctx, tx, id)
			outcome = out
			return err
		})
		if txErr != nil {
			switch {
			case errors.Is(txErr, ErrNotFound):
				return c.JSON(http.StatusNotFound, errBody("run not found"))
			case errors.Is(txErr, ErrAlreadyCancelling):
				return c.JSON(http.StatusConflict, errBody("run already cancelling"))
			case errors.Is(txErr, ErrTerminal):
				return c.JSON(http.StatusConflict, errBody("run already terminal"))
			}
			return c.JSON(http.StatusInternalServerError, errBody(txErr.Error()))
		}
		return c.JSON(http.StatusOK, outcome)
	}
}

func retryRunHandler(db *bun.DB, svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, errBody("invalid run id"))
		}
		var newRun *PipelineRun
		txErr := db.RunInTx(c.Request().Context(), nil, func(ctx context.Context, tx bun.Tx) error {
			r, err := svc.CreateRetry(ctx, tx, id)
			newRun = r
			return err
		})
		if txErr != nil {
			var nr *ErrNotRetryable
			switch {
			case errors.Is(txErr, ErrNotFound):
				return c.JSON(http.StatusNotFound, errBody("run not found"))
			case errors.As(txErr, &nr):
				return c.JSON(http.StatusConflict, errBody(nr.Error()))
			}
			return c.JSON(http.StatusInternalServerError, errBody(txErr.Error()))
		}
		retryOf := uuid.Nil
		if newRun.RetryOfRunID != nil {
			retryOf = *newRun.RetryOfRunID
		}
		return c.JSON(http.StatusCreated, RetryRunResponse{
			RunID:           newRun.ID,
			RetryOfRunID:    retryOf,
			Status:          newRun.Status,
			PipelineName:    newRun.PipelineName,
			PipelineVersion: newRun.PipelineVersion,
		})
	}
}

func resolveStepHandler(db *bun.DB, svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		runID, err := uuid.Parse(c.Param("id"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, errBody("invalid run id"))
		}
		stepName := c.Param("step")
		var body ResolveStepRequest
		if err := c.Bind(&body); err != nil {
			return c.JSON(http.StatusBadRequest, errBody(err.Error()))
		}
		var resolved bool
		txErr := db.RunInTx(c.Request().Context(), nil, func(ctx context.Context, tx bun.Tx) error {
			step, err := svc.LatestStepAttempt(ctx, tx, runID, stepName)
			if err != nil {
				return err
			}
			ok, err := svc.ResolveEventWaiter(ctx, tx, step.ID, body.Payload)
			resolved = ok
			return err
		})
		if txErr != nil {
			if errors.Is(txErr, ErrNotFound) {
				return c.JSON(http.StatusNotFound, errBody("step or waiter not found"))
			}
			return c.JSON(http.StatusInternalServerError, errBody(txErr.Error()))
		}
		if !resolved {
			return c.JSON(http.StatusConflict, errBody("waiter already resolved"))
		}
		return c.NoContent(http.StatusNoContent)
	}
}

// --- helpers --------------------------------------------------------

func parseInt(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return def
	}
	return v
}

func parseStatuses(values []string) []RunStatus {
	out := make([]RunStatus, 0, len(values))
	for _, v := range values {
		if v == "" {
			continue
		}
		out = append(out, RunStatus(v))
	}
	return out
}

func errBody(msg string) map[string]string {
	return map[string]string{"detail": msg}
}
