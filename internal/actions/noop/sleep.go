// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package noop

import (
	"fmt"
	"time"

	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/registry"
)

// SleepArgs is the input contract for noop.sleep.
//
// SleepMillis is bounded so a misconfigured pipeline can't park a
// worker indefinitely. 0 < ms <= 60_000.
type SleepArgs struct {
	SleepMillis int `json:"sleep_millis"`
}

// SleepResult reports the slept-for duration in millis.
type SleepResult struct {
	SleptMillis int `json:"slept_millis"`
}

const maxSleepMillis = 60_000

func sleep(args SleepArgs, ctx registry.ActionContext) (SleepResult, error) {
	if args.SleepMillis <= 0 {
		return SleepResult{}, fmt.Errorf("noop.sleep: sleep_millis must be > 0")
	}
	if args.SleepMillis > maxSleepMillis {
		return SleepResult{}, fmt.Errorf("noop.sleep: sleep_millis %d exceeds max %d", args.SleepMillis, maxSleepMillis)
	}
	select {
	case <-ctx.Ctx.Done():
		return SleepResult{}, ctx.Ctx.Err()
	case <-time.After(time.Duration(args.SleepMillis) * time.Millisecond):
		return SleepResult{SleptMillis: args.SleepMillis}, nil
	}
}
