// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package config defines the immutable bootstrap Settings tree and the
// Loader that materialises it from a secretmanagers.Manager.
//
// Layout convention: one file per section.
//
//	settings.go   the Settings aggregate (this file)
//	loader.go     Load() + decode helpers
//	postgres.go   Postgres section
//	rabbitmq.go   RabbitMQ section
//	app.go        App section
//	<name>.go     each future section goes in its own file
//
// Adding a section: create <name>.go with a struct, Default<Name>(),
// and load<Name>(sm). Wire it in Load(); nothing else changes.
//
// Pure value types: no env reads, no I/O. Defaults via Default* helpers
// (Go has no struct-default tag equivalent to pydantic's Field).
package config

// Settings is the top-level immutable bootstrap snapshot. Built once
// per process by Load and passed by value into every factory.
type Settings struct {
	Postgres   Postgres
	RabbitMQ   RabbitMQ
	App        App
	Cartridges Cartridges
}
