// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package secrets

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Canonical certificate formats. Mirrored by the CHECK constraint.
const (
	FormatX509    = "x509"
	FormatOpenSSH = "openssh"
)

// Canonical certificate/key usages. A certificate may carry several
// (X.509 EKU is a set; an SSH key has one role), so Usage is an array.
const (
	UsageTLS            = "tls"
	UsageSSHUserAuth    = "ssh_user_auth"
	UsageSSHHostAuth    = "ssh_host_auth"
	UsageSAMLSigning    = "saml_signing"
	UsageSAMLEncryption = "saml_encryption"
)

// SecretCertificate is one piece of PKI material — an X.509 or OpenSSH
// certificate/key.
//
// Natural key: (Source, ExternalID). The locus edge and principal_id
// follow the same rules as SecretPlain.
//
// Usage is the certificate's role set. Subject / Issuer / Serial /
// Fingerprint / KeyAlgorithm / KeySize / IsCA / SelfSigned are the
// structured PKI facts the posture cartridge reasons over (weak key,
// self-signed, long validity, expiring). Fingerprint here is the public
// thumbprint of the certificate, not a value hash.
//
// NotBefore / NotAfter are the validity window (NotAfter = expiry); a
// NULL value means the source never evidenced it (a Blind Spot).
type SecretCertificate struct {
	bun.BaseModel `bun:"table:secret_certificate,alias:sc"`

	ID                   uuid.UUID      `bun:"id,pk,type:uuid"               json:"id"`
	ExternalID           string         `bun:"external_id,notnull"           json:"external_id"`
	Source               string         `bun:"source,notnull"                json:"source"`
	Format               string         `bun:"format,notnull"                json:"format"`
	Usage                []string       `bun:"usage,type:jsonb,notnull"      json:"usage"`
	Label                string         `bun:"label,notnull"                 json:"label"`
	TargetApplicationID  *uuid.UUID     `bun:"target_application_id,type:uuid"   json:"target_application_id,omitempty"`
	AccountID            *uuid.UUID     `bun:"account_id,type:uuid"          json:"account_id,omitempty"`
	FoundInApplicationID *uuid.UUID     `bun:"found_in_application_id,type:uuid" json:"found_in_application_id,omitempty"`
	FoundInLocation      *string        `bun:"found_in_location"             json:"found_in_location,omitempty"`
	PrincipalID          *uuid.UUID     `bun:"principal_id,type:uuid"        json:"principal_id,omitempty"`
	Subject              *string        `bun:"subject"                       json:"subject,omitempty"`
	Issuer               *string        `bun:"issuer"                        json:"issuer,omitempty"`
	Serial               *string        `bun:"serial"                        json:"serial,omitempty"`
	Fingerprint          *string        `bun:"fingerprint"                   json:"fingerprint,omitempty"`
	KeyAlgorithm         *string        `bun:"key_algorithm"                 json:"key_algorithm,omitempty"`
	KeySize              *int           `bun:"key_size"                      json:"key_size,omitempty"`
	IsCA                 bool           `bun:"is_ca,notnull"                 json:"is_ca"`
	SelfSigned           bool           `bun:"self_signed,notnull"           json:"self_signed"`
	IsActive             bool           `bun:"is_active,notnull"             json:"is_active"`
	IsPrivileged         bool           `bun:"is_privileged,notnull"         json:"is_privileged"`
	NotBefore            *time.Time     `bun:"not_before"                    json:"not_before,omitempty"`
	NotAfter             *time.Time     `bun:"not_after"                     json:"not_after,omitempty"`
	LastUsedAt           *time.Time     `bun:"last_used_at"                  json:"last_used_at,omitempty"`
	Attrs                map[string]any `bun:"attrs,type:jsonb,notnull"  json:"attrs"`
	CreatedAt            time.Time      `bun:"created_at,notnull"            json:"created_at"`
	UpdatedAt            time.Time      `bun:"updated_at,notnull"            json:"updated_at"`
}

// CertListFilter narrows what ListCertificates returns. Zero value lists
// every row.
type CertListFilter struct {
	TargetApplicationID  *uuid.UUID
	FoundInApplicationID *uuid.UUID
	AccountID            *uuid.UUID
	PrincipalID          *uuid.UUID
	Format               string
	Privileged           *bool
	Linked               *bool
	Limit                int
	Offset               int
}

// CertRepository is the persistence boundary for SecretCertificate.
type CertRepository interface {
	Upsert(ctx context.Context, tx bun.IDB, c *SecretCertificate) error
	List(ctx context.Context, tx bun.IDB, f CertListFilter) ([]*SecretCertificate, int, error)
}

// CertLookup resolves a single certificate by id.
type CertLookup interface {
	GetByID(ctx context.Context, tx bun.IDB, id uuid.UUID) (*SecretCertificate, error)
}

// CertBunRepository is the production Postgres-backed implementation of
// both CertRepository and CertLookup.
type CertBunRepository struct{}

// NewCertBunRepository constructs a CertBunRepository.
func NewCertBunRepository() *CertBunRepository { return &CertBunRepository{} }

// Upsert inserts a new SecretCertificate or updates the existing one
// keyed by (source, external_id). updated_at is always refreshed.
func (r *CertBunRepository) Upsert(ctx context.Context, tx bun.IDB, c *SecretCertificate) error {
	if c.Usage == nil {
		c.Usage = []string{}
	}
	if c.Attrs == nil {
		c.Attrs = map[string]any{}
	}
	_, err := tx.NewInsert().
		Model(c).
		On("CONFLICT (source, external_id) DO UPDATE").
		Set("format                  = EXCLUDED.format").
		Set("usage                   = EXCLUDED.usage").
		Set("label                   = EXCLUDED.label").
		Set("target_application_id   = EXCLUDED.target_application_id").
		Set("account_id              = EXCLUDED.account_id").
		Set("found_in_application_id = EXCLUDED.found_in_application_id").
		Set("found_in_location       = EXCLUDED.found_in_location").
		Set("principal_id            = EXCLUDED.principal_id").
		Set("subject                 = EXCLUDED.subject").
		Set("issuer                  = EXCLUDED.issuer").
		Set("serial                  = EXCLUDED.serial").
		Set("fingerprint             = EXCLUDED.fingerprint").
		Set("key_algorithm           = EXCLUDED.key_algorithm").
		Set("key_size                = EXCLUDED.key_size").
		Set("is_ca                   = EXCLUDED.is_ca").
		Set("self_signed             = EXCLUDED.self_signed").
		Set("is_active               = EXCLUDED.is_active").
		Set("is_privileged           = EXCLUDED.is_privileged").
		Set("not_before              = EXCLUDED.not_before").
		Set("not_after               = EXCLUDED.not_after").
		Set("last_used_at            = EXCLUDED.last_used_at").
		Set("attrs                   = EXCLUDED.attrs").
		Set("updated_at              = EXCLUDED.updated_at").
		Exec(ctx)
	return err
}

// List returns a paginated snapshot plus total row count honouring the
// filter. Default ordering is created_at ASC for a deterministic walk.
func (r *CertBunRepository) List(ctx context.Context, tx bun.IDB, f CertListFilter) ([]*SecretCertificate, int, error) {
	out := []*SecretCertificate{}
	q := tx.NewSelect().Model(&out)
	if f.TargetApplicationID != nil {
		q = q.Where("target_application_id = ?", *f.TargetApplicationID)
	}
	if f.FoundInApplicationID != nil {
		q = q.Where("found_in_application_id = ?", *f.FoundInApplicationID)
	}
	if f.AccountID != nil {
		q = q.Where("account_id = ?", *f.AccountID)
	}
	if f.PrincipalID != nil {
		q = q.Where("principal_id = ?", *f.PrincipalID)
	}
	if f.Format != "" {
		q = q.Where("format = ?", f.Format)
	}
	if f.Privileged != nil {
		q = q.Where("is_privileged = ?", *f.Privileged)
	}
	if f.Linked != nil {
		if *f.Linked {
			q = q.Where("principal_id IS NOT NULL")
		} else {
			q = q.Where("principal_id IS NULL")
		}
	}
	q = q.Order("created_at ASC")
	if f.Limit > 0 {
		q = q.Limit(f.Limit)
	}
	if f.Offset > 0 {
		q = q.Offset(f.Offset)
	}
	total, err := q.ScanAndCount(ctx)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// GetByID returns the SecretCertificate with the given primary key, or
// ErrNotFound.
func (r *CertBunRepository) GetByID(ctx context.Context, tx bun.IDB, id uuid.UUID) (*SecretCertificate, error) {
	c := new(SecretCertificate)
	err := tx.NewSelect().Model(c).Where("id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return c, nil
}
