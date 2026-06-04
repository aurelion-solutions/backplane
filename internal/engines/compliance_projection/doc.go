// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package compliance_projection projects identity-posture findings onto
// external compliance control languages (SOC 2 logical access, …).
//
// A projection is a VIEW over a single assessment run, never a source of
// truth. The engine carries no policies of its own: it reads a
// declarative projection definition from a cartridge (control → the
// finding kinds that violate it) and rolls the run's existing findings
// and policy-evaluation outcomes up into a per-control coverage state.
//
// Coverage is honest about what cannot be proven. A control is
// `covered` only when its population was actually evaluated AND no
// violating finding exists. An unevaluated population or an
// evidence gap on a rule the control depends on yields `not_evaluable`
// — never a silent green tick.
//
//	covered        evaluated, zero violations, zero gaps
//	failed         >=1 violation, zero gaps
//	partial        >=1 violation AND >=1 gap
//	not_evaluable  zero violations with gaps, OR population never evaluated
//
// The engine computes read-time over the run's findings/outcomes; it
// persists nothing. The optional evidence packet (export) is a pure
// serialisation of the same computed coverage.
package compliance_projection
