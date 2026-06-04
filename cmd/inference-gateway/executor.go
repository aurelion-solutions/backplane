// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"context"
	"fmt"

	"github.com/aurelion-solutions/backplane/internal/core/config"
	"github.com/aurelion-solutions/backplane/internal/platform/llm"
)

// Executor turns one StreamRequest into a stream of chunks. It is the
// gateway's swap point: today the only implementation is LocalExecutor
// (in-process provider via the llm factory). When GPU slots need
// scaling, a DistributedExecutor fanning out to an inference-worker
// pool slots in behind this same interface — the gateway's HTTP
// contract does not change.
type Executor interface {
	Stream(ctx context.Context, req StreamRequest) (<-chan llm.Chunk, error)
}

// LocalExecutor resolves the requested named backend to a protocol +
// Config through config.LLM, builds the provider from the factory, and
// streams in-process. No worker pool, no scheduler.
type LocalExecutor struct {
	factory *llm.Factory
	cfg     config.LLM
}

// NewLocalExecutor wires a LocalExecutor over an already-populated
// factory and the loaded LLM config.
func NewLocalExecutor(factory *llm.Factory, cfg config.LLM) *LocalExecutor {
	return &LocalExecutor{factory: factory, cfg: cfg}
}

// resolve picks the named provider entry, or the configured default
// when name is empty.
func (e *LocalExecutor) resolve(name string) (config.LLMProvider, error) {
	if name == "" {
		return e.cfg.Active()
	}
	p, ok := e.cfg.Providers[name]
	if !ok {
		return config.LLMProvider{}, fmt.Errorf("inference: provider %q not found in config", name)
	}
	return p, nil
}

// Stream implements Executor.
func (e *LocalExecutor) Stream(ctx context.Context, req StreamRequest) (<-chan llm.Chunk, error) {
	entry, err := e.resolve(req.Provider)
	if err != nil {
		return nil, err
	}
	provider, err := e.factory.Get(entry.Protocol, llm.Config{
		BaseURL: entry.BaseURL,
		APIKey:  entry.APIKey,
		Model:   entry.Model,
	})
	if err != nil {
		return nil, err
	}
	return provider.Stream(ctx, req.Messages, req.Params)
}
