// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package secrets owns secret material — authentication evidence that
// is NOT an identity. It is split into two entities by shape:
//
//   - SecretPlain: opaque material (passwords, connection strings,
//     tokens, API keys). Carries an optional value fingerprint for
//     reuse/leak correlation, plus token scopes.
//   - SecretCertificate: PKI material (X.509 / OpenSSH certs and keys).
//     Carries format + usage[] and the structured PKI fields the
//     posture cartridge queries (validity window, key size, issuer,
//     self-signed).
//
// A secret is an EDGE, not a node. It links where it was found (a
// client application / config / vault — found_in_*) to what it
// authenticates to (target_application_id + account_id). Discovery is
// messy: a token is usually found in the receiving application itself,
// while a password / connection string / key is found in a client app's
// config. Both ends and the locus are therefore nullable; a storage
// CHECK forbids a secret with no locus at all.
//
// principal_id is the owner/subject, independent of either end. NULL =
// unresolved linkage — a posture signal, never a reason to promote a
// secret into an identity of its own. A system token vs a PAT is not a
// stored sub-kind; it is read from the linked principal's kind
// (workload ⇒ system, employment ⇒ PAT).
package secrets
