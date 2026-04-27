// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package inventory_ingest

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/aurelion-solutions/backplane/internal/core/correlation"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// HTTPRequest is the POST /ingest body. Mirrors Request but with json
// tags so echo.Bind decodes it.
type HTTPRequest struct {
	Source        string           `json:"source"`
	DatasetType   string           `json:"dataset_type"`
	CorrelationID string           `json:"correlation_id,omitempty"`
	Records       []map[string]any `json:"records"`
}

// RegisterRoutes mounts the ingest HTTP surface on g.
func RegisterRoutes(g *echo.Group, svc *Service) {
	g.POST("/ingest", processHandler(svc))
	g.GET("/ingest/batches", listHandler(svc))
	g.GET("/ingest/batches/:id", getHandler(svc))
}

func processHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		var in HTTPRequest
		if err := c.Bind(&in); err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		cid := strings.TrimSpace(in.CorrelationID)
		if cid == "" {
			cid = strings.TrimSpace(c.Request().Header.Get("X-Correlation-Id"))
		}
		ctx := c.Request().Context()
		if cid != "" {
			ctx = correlation.WithID(ctx, cid)
		}
		result, err := svc.Process(ctx, Request{
			Source:        in.Source,
			DatasetType:   in.DatasetType,
			CorrelationID: cid,
			Records:       in.Records,
		})
		if err != nil {
			switch {
			case errors.Is(err, ErrInvalidEnvelope),
				errors.Is(err, ErrMissingExternalID),
				errors.Is(err, ErrEmptyRecords),
				errors.Is(err, ErrBatchTooLarge):
				return c.JSON(http.StatusBadRequest, errorBody(err))
			}
			return c.JSON(http.StatusBadGateway, errorBody(err))
		}
		return c.JSON(http.StatusCreated, result)
	}
}

func listHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		limit := parseIntDefault(c.QueryParam("limit"), 100)
		offset := parseIntDefault(c.QueryParam("offset"), 0)
		items, total, err := svc.List(c.Request().Context(), limit, offset)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, errorBody(err))
		}
		return c.JSON(http.StatusOK, ListResponse{Items: items, Total: total, Limit: limit, Offset: offset})
	}
}

func getHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		row, err := svc.Get(c.Request().Context(), id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return c.JSON(http.StatusNotFound, errorBody(err))
			}
			return c.JSON(http.StatusInternalServerError, errorBody(err))
		}
		return c.JSON(http.StatusOK, row)
	}
}

func parseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return def
	}
	return v
}

func errorBody(err error) map[string]string {
	return map[string]string{"detail": err.Error()}
}
