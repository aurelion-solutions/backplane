// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package runner

import (
	"fmt"
	"os"
	"sync/atomic"
)

// WorkerIdentity is the immutable identity of one runner slot.
//
// Format: "<hostname>-<pid>-<slot_index>" — same string stored in
// pipeline_runs.worker_id. Tags are shared across every slot of the
// same process — they come from AURELION_WORKER_TAGS at startup.
type WorkerIdentity struct {
	Hostname  string
	PID       int
	SlotIndex int
	WorkerID  string
	Tags      []string
}

// NewWorkerIdentity builds an identity for the current process.
func NewWorkerIdentity(slotIndex int, tags []string) WorkerIdentity {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "unknown"
	}
	pid := os.Getpid()
	// Defensive copy + ensure non-nil so the SQL array column gets `{}`
	// instead of NULL via bun.
	t := append([]string{}, tags...)
	return WorkerIdentity{
		Hostname:  hostname,
		PID:       pid,
		SlotIndex: slotIndex,
		WorkerID:  fmt.Sprintf("%s-%d-%d", hostname, pid, slotIndex),
		Tags:      t,
	}
}

// runHandle is the shared coordination state between RunOneIteration
// and the surrounding work loop. Used by drain on shutdown to know
// which run (if any) is in-flight.
type runHandle struct {
	runID atomic.Value // uuid.UUID; zero when idle
	done  chan struct{}
}

func newRunHandle() *runHandle {
	return &runHandle{done: make(chan struct{}, 1)}
}
