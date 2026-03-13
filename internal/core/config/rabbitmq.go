// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package config

import (
	"fmt"
	"net/url"

	"github.com/aurelion-solutions/backplane/internal/core/secret"
)

// RabbitMQ holds AMQP connection parameters and exchange names.
type RabbitMQ struct {
	Host                       string `json:"host"`
	Port                       int    `json:"port"`
	Username                   string `json:"username"`
	Password                   string `json:"password"`
	EventsExchange             string `json:"events_exchange"`
	LogsExchange               string `json:"logs_exchange"`
	ConnectorCommandsExchange  string `json:"connector_commands_exchange"`
	ConnectorResponsesExchange string `json:"connector_responses_exchange"`
}

// DefaultRabbitMQ returns local-dev defaults aligned with kernel.
func DefaultRabbitMQ() RabbitMQ {
	return RabbitMQ{
		Host:                       "localhost",
		Port:                       5672,
		Username:                   "guest",
		Password:                   "guest",
		EventsExchange:             "aurelion.events",
		LogsExchange:               "aurelion.logs",
		ConnectorCommandsExchange:  "aurelion.connectors.commands",
		ConnectorResponsesExchange: "aurelion.connectors.responses",
	}
}

// URL renders an AMQP URI suitable for amqp.Dial.
func (r RabbitMQ) URL() string {
	u := &url.URL{
		Scheme: "amqp",
		User:   url.UserPassword(r.Username, r.Password),
		Host:   fmt.Sprintf("%s:%d", r.Host, r.Port),
		Path:   "/",
	}
	return u.String()
}

func loadRabbitMQ(sm secret.Manager) (RabbitMQ, error) {
	r := DefaultRabbitMQ()
	if err := decodeRequired(sm, "rabbitmq", &r); err != nil {
		return RabbitMQ{}, err
	}
	return r, nil
}
