// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package finding_explanation is the L2 engine that turns an already
// proven finding into a human-readable narrative. It explains findings;
// it never creates them.
//
// The deterministic boundary is the whole point. The finding, its
// severity, its evidence chain and the policy that produced it are
// facts decided upstream by the assessment engines. This engine only
// packages those facts into prose. It cannot mint a finding, a score, a
// severity or a remediation — every claim the model emits must point
// back to evidence, policy, finding or source that was in the input.
//
// Shape of one explanation:
//
//	collect context (finding + evidence chain + policy)  — read-only
//	    -> render prompt + compute input_hash
//	    -> cache check (finding_id, input_hash)           — reuse if fresh
//	    -> inference-gateway (model execution)            — over a port
//	    -> validate citations against the input refs      — drop strays
//	    -> persist explanation artifact                   — durable, cited
//
// Why persisted (unlike compliance_projection, which is read-time):
// generation is expensive (GPU seconds) and must be cached and cited, so
// the explanation is an artifact, not a view. The cache key is
// input_hash — the same finding / evidence / policy / template / model
// does not regenerate; a change in any of them invalidates.
//
// What lives elsewhere. Model execution and streaming transport live in
// cmd/inference-gateway, reached here through the InferenceClient port —
// this engine never wires a provider of its own. The MQ path (durable /
// bulk background generation) is a future executor, not part of this
// engine. There is no tenant scoping, quota subsystem, or cancellation
// machinery here — those are out of scope until the platform grows the
// corresponding primitives.
package finding_explanation
