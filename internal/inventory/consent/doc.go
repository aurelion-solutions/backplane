// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package consent owns delegated-access evidence: the applications that
// identities have consented to, and the grants that record it. A consent
// grant is EVIDENCE of delegated access, not identity truth.
//
// The slice is split into two entities:
//
//   - ConsentedApplication: the application as it PRESENTED itself in a
//     consent flow. Keyed on the verifiable anchor (Source, ClientID) —
//     the identifier the IdP issued. DisplayName / Publisher / HomeTenant
//     / RedirectURIs are claims the app asserts about itself and are not
//     trusted; VerifiedPublisher is the single datum the IdP actually
//     confirmed. A resolver may link the presented app to a governed
//     identity (ResolvedPrincipalID + ResolutionConfidence); Origin is
//     derived from that resolution. An unresolved app is never promoted
//     into a principal of its own — it stays a posture signal.
//   - ConsentGrant: the fact that some subject granted a presented app a
//     set of scopes. Scopes live HERE, not on the application, because
//     one app receives different scopes from different subjects. Scopes
//     are stored raw and unclassified — "high risk" is a policy verdict
//     emitted by the posture cartridge, never a stored fact. A NULL
//     ConsentingPrincipalID is tenant-wide admin consent or an
//     unresolved owner. A NULL LastUsedAt makes a staleness check
//     not_evaluable (a Blind Spot), not a silent pass.
//
// The key product stance: we do not trust an application's self-asserted
// name. The inventory shows where you cannot say who you actually
// granted access to.
package consent
