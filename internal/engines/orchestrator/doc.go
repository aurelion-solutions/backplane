// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package orchestrator runs declarative pipelines (a list of Steps)
// against a set of executors. It is a self-contained engine — its
// only inbound dependencies are core/* and platform/* contracts; no
// other engine reaches into it.
//
// Two binaries share this code:
//
//	cmd/backplane  — registers HTTP routes (POST /runs, GET /runs/{id})
//	                 and a matcher MQ consumer that hands Steps off to
//	                 worker nodes via the broker.
//	cmd/worker   — long-running runner loop that competes for Steps
//	                 from the matcher and reports back over MQ.
//
// This file is a skeleton. Service, Runner and persistence are stubs
// that return ErrNotImplemented; real implementations land slice by
// slice as the orchestrator surface stabilises.
package orchestrator
