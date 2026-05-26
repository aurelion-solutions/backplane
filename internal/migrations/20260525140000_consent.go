// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package migrations

import (
	"context"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(
		func(ctx context.Context, db *bun.DB) error {
			// A consent grant is EVIDENCE of delegated access, not identity
			// truth. The application on the receiving end presents itself in
			// the consent flow, and only the IdP-issued anchor (client_id /
			// app_id) is trustworthy — display_name and publisher are claims
			// the app asserts about itself. So we model two things:
			//
			//   consented_application — the presented client, keyed on the
			//                           verifiable anchor (source, client_id).
			//                           display_name/publisher/home_tenant/
			//                           redirect_uris are untrusted claims;
			//                           verified_publisher is the one datum the
			//                           IdP actually confirmed. A resolver may
			//                           link it to a governed identity, recorded
			//                           as resolved_principal_id +
			//                           resolution_confidence; origin is derived
			//                           from that resolution. We never mint a
			//                           principal for a presented app — an
			//                           unresolved app stays unresolved, a
			//                           posture signal, never an identity.
			//   consent_grant         — the fact that some subject granted this
			//                           app a set of scopes. Scopes live HERE
			//                           (one app gets different scopes from
			//                           different subjects), raw and
			//                           unclassified; "high risk" is a policy
			//                           verdict, not a stored fact. NULL
			//                           consenting_principal_id = tenant-wide
			//                           admin consent or an unresolved owner.
			//                           NULL last_used_at makes a staleness
			//                           check not_evaluable (a Blind Spot),
			//                           never a silent pass.
			_, err := db.ExecContext(ctx, `
				CREATE TABLE IF NOT EXISTS consented_application (
					id                    uuid PRIMARY KEY,
					source                text NOT NULL,
					client_id             text NOT NULL,
					app_id                text,
					display_name          text,
					publisher             text,
					verified_publisher    boolean NOT NULL DEFAULT FALSE,
					home_tenant           text,
					redirect_uris         jsonb NOT NULL DEFAULT '[]'::jsonb,
					resolved_principal_id uuid REFERENCES principals(id) ON DELETE SET NULL,
					resolution_confidence text NOT NULL DEFAULT 'unresolved',
					origin                text NOT NULL DEFAULT 'unknown',
					is_active             boolean NOT NULL DEFAULT TRUE,
					attrs                 jsonb NOT NULL DEFAULT '{}'::jsonb,
					created_at            timestamptz NOT NULL DEFAULT NOW(),
					updated_at            timestamptz NOT NULL DEFAULT NOW(),
					CONSTRAINT consented_application_resolution_check
						CHECK (resolution_confidence IN
							('resolved','likely_same','ambiguous','unresolved','spoofing_suspected')),
					CONSTRAINT consented_application_origin_check
						CHECK (origin IN ('first_party','third_party','unknown')),
					CONSTRAINT consented_application_source_client_key
						UNIQUE (source, client_id)
				);
				CREATE INDEX IF NOT EXISTS idx_consented_application_principal
					ON consented_application (resolved_principal_id) WHERE resolved_principal_id IS NOT NULL;
				CREATE INDEX IF NOT EXISTS idx_consented_application_origin
					ON consented_application (origin);
				CREATE INDEX IF NOT EXISTS idx_consented_application_resolution
					ON consented_application (resolution_confidence);

				CREATE TABLE IF NOT EXISTS consent_grant (
					id                       uuid PRIMARY KEY,
					source                   text NOT NULL,
					external_id              text NOT NULL,
					consented_application_id uuid NOT NULL REFERENCES consented_application(id) ON DELETE CASCADE,
					consenting_principal_id  uuid REFERENCES principals(id) ON DELETE SET NULL,
					grant_type               text NOT NULL,
					scopes                   jsonb NOT NULL DEFAULT '[]'::jsonb,
					is_active                boolean NOT NULL DEFAULT TRUE,
					granted_at               timestamptz,
					expires_at               timestamptz,
					revoked_at               timestamptz,
					last_used_at             timestamptz,
					attrs                    jsonb NOT NULL DEFAULT '{}'::jsonb,
					created_at               timestamptz NOT NULL DEFAULT NOW(),
					updated_at               timestamptz NOT NULL DEFAULT NOW(),
					CONSTRAINT consent_grant_grant_type_check
						CHECK (grant_type IN ('delegated','application')),
					CONSTRAINT consent_grant_source_external_key
						UNIQUE (source, external_id)
				);
				CREATE INDEX IF NOT EXISTS idx_consent_grant_app
					ON consent_grant (consented_application_id);
				CREATE INDEX IF NOT EXISTS idx_consent_grant_subject
					ON consent_grant (consenting_principal_id) WHERE consenting_principal_id IS NOT NULL;

				-- Findings target the presented app or the grant itself, so
				-- widen the discriminated target to admit both kinds.
				ALTER TABLE findings DROP CONSTRAINT IF EXISTS findings_target_type_check;
				ALTER TABLE findings
					ADD CONSTRAINT findings_target_type_check
					CHECK (target_type IS NULL OR target_type IN
						('account','workload','secret_plain','secret_certificate',
						 'consented_application','consent_grant'));

				-- Parity with the secret pass: an evidence chain may normalize
				-- onto a consent artifact.
				ALTER TABLE evidence_chains
					DROP CONSTRAINT IF EXISTS evidence_chains_normalized_kind_check;
				ALTER TABLE evidence_chains
					ADD CONSTRAINT evidence_chains_normalized_kind_check
					CHECK (normalized_kind IS NULL OR normalized_kind IN
						('person','employment','account','workload',
						 'secret_plain','secret_certificate',
						 'consented_application','consent_grant'));
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
				ALTER TABLE evidence_chains
					DROP CONSTRAINT IF EXISTS evidence_chains_normalized_kind_check;
				ALTER TABLE evidence_chains
					ADD CONSTRAINT evidence_chains_normalized_kind_check
					CHECK (normalized_kind IS NULL OR normalized_kind IN
						('person','employment','account','workload',
						 'secret_plain','secret_certificate'));

				ALTER TABLE findings DROP CONSTRAINT IF EXISTS findings_target_type_check;
				ALTER TABLE findings
					ADD CONSTRAINT findings_target_type_check
					CHECK (target_type IS NULL OR target_type IN
						('account','workload','secret_plain','secret_certificate'));

				DROP TABLE IF EXISTS consent_grant;
				DROP TABLE IF EXISTS consented_application;
			`)
			return err
		},
	)
}
