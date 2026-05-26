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
			// Secrets are authentication evidence, NOT identities. They
			// are split into two tables by SHAPE, not by lifecycle stage:
			//
			//   secret_plain        — opaque material: passwords, connection
			//                         strings, tokens, API keys. No PKI
			//                         structure; carries an optional value
			//                         fingerprint (hash of the secret value)
			//                         for reuse/leak correlation.
			//   secret_certificate  — PKI material: X.509 / OpenSSH certs and
			//                         keys. Carries format + usage[] + the
			//                         structured PKI fields posture checks
			//                         query (validity window, key size,
			//                         self-signed, ...).
			//
			// A secret is an EDGE between where it was found (a client app /
			// config / vault) and what it authenticates to (the target app
			// + account). Discovery is messy, so both ends and the locus are
			// nullable; a CHECK forbids a secret with no locus at all.
			// principal_id is the owner/subject, independent of either end;
			// NULL = unresolved linkage, a posture signal, never a reason to
			// promote the secret into an identity.
			_, err := db.ExecContext(ctx, `
				CREATE TABLE IF NOT EXISTS secret_plain (
					id                      uuid PRIMARY KEY,
					external_id             text NOT NULL,
					source                  text NOT NULL,
					type                    text NOT NULL,
					label                   text NOT NULL,
					target_application_id   uuid REFERENCES applications(id) ON DELETE SET NULL,
					account_id              uuid REFERENCES accounts(id)     ON DELETE SET NULL,
					found_in_application_id uuid REFERENCES applications(id) ON DELETE SET NULL,
					found_in_location       text,
					principal_id            uuid REFERENCES principals(id)   ON DELETE SET NULL,
					scopes                  jsonb NOT NULL DEFAULT '[]'::jsonb,
					fingerprint             text,
					is_active               boolean NOT NULL DEFAULT TRUE,
					is_privileged           boolean NOT NULL DEFAULT FALSE,
					issued_at               timestamptz,
					expires_at              timestamptz,
					rotated_at              timestamptz,
					last_used_at            timestamptz,
					attrs                   jsonb NOT NULL DEFAULT '{}'::jsonb,
					created_at              timestamptz NOT NULL DEFAULT NOW(),
					updated_at              timestamptz NOT NULL DEFAULT NOW(),
					CONSTRAINT secret_plain_type_check
						CHECK (type IN ('password','connstring','token','api_key')),
					CONSTRAINT secret_plain_locus_check
						CHECK (target_application_id IS NOT NULL
							OR found_in_application_id IS NOT NULL
							OR found_in_location IS NOT NULL),
					CONSTRAINT secret_plain_source_external_key
						UNIQUE (source, external_id)
				);
				CREATE INDEX IF NOT EXISTS idx_secret_plain_target_app
					ON secret_plain (target_application_id) WHERE target_application_id IS NOT NULL;
				CREATE INDEX IF NOT EXISTS idx_secret_plain_found_in_app
					ON secret_plain (found_in_application_id) WHERE found_in_application_id IS NOT NULL;
				CREATE INDEX IF NOT EXISTS idx_secret_plain_account
					ON secret_plain (account_id) WHERE account_id IS NOT NULL;
				CREATE INDEX IF NOT EXISTS idx_secret_plain_principal
					ON secret_plain (principal_id) WHERE principal_id IS NOT NULL;

				CREATE TABLE IF NOT EXISTS secret_certificate (
					id                      uuid PRIMARY KEY,
					external_id             text NOT NULL,
					source                  text NOT NULL,
					format                  text NOT NULL,
					usage                   jsonb NOT NULL DEFAULT '[]'::jsonb,
					label                   text NOT NULL,
					target_application_id   uuid REFERENCES applications(id) ON DELETE SET NULL,
					account_id              uuid REFERENCES accounts(id)     ON DELETE SET NULL,
					found_in_application_id uuid REFERENCES applications(id) ON DELETE SET NULL,
					found_in_location       text,
					principal_id            uuid REFERENCES principals(id)   ON DELETE SET NULL,
					subject                 text,
					issuer                  text,
					serial                  text,
					fingerprint             text,
					key_algorithm           text,
					key_size                integer,
					is_ca                   boolean NOT NULL DEFAULT FALSE,
					self_signed             boolean NOT NULL DEFAULT FALSE,
					is_active               boolean NOT NULL DEFAULT TRUE,
					is_privileged           boolean NOT NULL DEFAULT FALSE,
					not_before              timestamptz,
					not_after               timestamptz,
					last_used_at            timestamptz,
					attrs                   jsonb NOT NULL DEFAULT '{}'::jsonb,
					created_at              timestamptz NOT NULL DEFAULT NOW(),
					updated_at              timestamptz NOT NULL DEFAULT NOW(),
					CONSTRAINT secret_certificate_format_check
						CHECK (format IN ('x509','openssh')),
					CONSTRAINT secret_certificate_locus_check
						CHECK (target_application_id IS NOT NULL
							OR found_in_application_id IS NOT NULL
							OR found_in_location IS NOT NULL),
					CONSTRAINT secret_certificate_source_external_key
						UNIQUE (source, external_id)
				);
				CREATE INDEX IF NOT EXISTS idx_secret_certificate_target_app
					ON secret_certificate (target_application_id) WHERE target_application_id IS NOT NULL;
				CREATE INDEX IF NOT EXISTS idx_secret_certificate_found_in_app
					ON secret_certificate (found_in_application_id) WHERE found_in_application_id IS NOT NULL;
				CREATE INDEX IF NOT EXISTS idx_secret_certificate_account
					ON secret_certificate (account_id) WHERE account_id IS NOT NULL;
				CREATE INDEX IF NOT EXISTS idx_secret_certificate_principal
					ON secret_certificate (principal_id) WHERE principal_id IS NOT NULL;
			`)
			return err
		},
		func(ctx context.Context, db *bun.DB) error {
			_, err := db.ExecContext(ctx, `
				DROP TABLE IF EXISTS secret_certificate;
				DROP TABLE IF EXISTS secret_plain;
			`)
			return err
		},
	)
}
