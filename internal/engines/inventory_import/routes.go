// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package inventory_import

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/aurelion-solutions/backplane/internal/engines/inventory_ingest"
)

// RegisterRoutes mounts the synchronous import endpoint on g.
//
// POST /inventory/import — body is HTTPRequest, response on success
// is HTTPResponse + 200. Validation errors (bad dataset_type, empty
// records, missing external_id, etc.) come back as 400; everything
// else as 500.
func RegisterRoutes(g *echo.Group, svc *Service) {
	g.POST("/inventory/import", processHandler(svc))
}

func processHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		var in HTTPRequest
		if err := c.Bind(&in); err != nil {
			return c.JSON(http.StatusBadRequest, errorBody(err))
		}
		out, err := svc.Process(c.Request().Context(), in)
		if err != nil {
			switch {
			case errors.Is(err, inventory_ingest.ErrInvalidEnvelope),
				errors.Is(err, inventory_ingest.ErrMissingExternalID),
				errors.Is(err, inventory_ingest.ErrEmptyRecords),
				errors.Is(err, inventory_ingest.ErrBatchTooLarge):
				return c.JSON(http.StatusBadRequest, errorBody(err))
			}
			return c.JSON(http.StatusInternalServerError, errorBody(err))
		}
		return c.JSON(http.StatusOK, out)
	}
}

func errorBody(err error) map[string]string {
	return map[string]string{"detail": err.Error()}
}
