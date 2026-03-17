// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package connectors

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
)

// InstanceWire is the JSON shape returned by the connector-instances
// endpoints. Kept exported because cross-slice adapters (e.g. the
// applications matching endpoint) need to serialise the same shape.
type InstanceWire struct {
	ID         string   `json:"id"`
	InstanceID string   `json:"instance_id"`
	Tags       []string `json:"tags"`
	IsOnline   bool     `json:"is_online"`
	LastSeenAt string   `json:"last_seen_at"`
	CreatedAt  string   `json:"created_at"`
	UpdatedAt  string   `json:"updated_at"`
}

// NewInstanceWire converts a domain instance into the JSON shape.
func NewInstanceWire(inst *ConnectorInstance) InstanceWire {
	return InstanceWire{
		ID:         inst.ID.String(),
		InstanceID: inst.InstanceID,
		Tags:       inst.Tags,
		IsOnline:   inst.IsOnline(),
		LastSeenAt: inst.LastSeenAt.UTC().Format("2006-01-02T15:04:05.999999Z07:00"),
		CreatedAt:  inst.CreatedAt.UTC().Format("2006-01-02T15:04:05.999999Z07:00"),
		UpdatedAt:  inst.UpdatedAt.UTC().Format("2006-01-02T15:04:05.999999Z07:00"),
	}
}

// RegisterRoutes mounts /connector-instances under g.
func RegisterRoutes(g *echo.Group, svc *Service) {
	g.GET("/connector-instances", listHandler(svc))
	g.GET("/connector-instances/:instance_id", getHandler(svc))
}

func listHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		instances, err := svc.List(c.Request().Context())
		if err != nil {
			return c.JSON(http.StatusInternalServerError, errorBody(err))
		}
		out := make([]InstanceWire, 0, len(instances))
		for _, inst := range instances {
			out = append(out, NewInstanceWire(inst))
		}
		return c.JSON(http.StatusOK, out)
	}
}

func getHandler(svc *Service) echo.HandlerFunc {
	return func(c echo.Context) error {
		inst, err := svc.Get(c.Request().Context(), c.Param("instance_id"))
		if err != nil {
			if errors.Is(err, ErrInstanceNotFound) {
				return c.JSON(http.StatusNotFound, errorBody(err))
			}
			return c.JSON(http.StatusInternalServerError, errorBody(err))
		}
		return c.JSON(http.StatusOK, NewInstanceWire(inst))
	}
}

func errorBody(err error) map[string]string {
	return map[string]string{"detail": err.Error()}
}
