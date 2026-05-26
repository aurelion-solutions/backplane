# Changelog

All notable changes to this project are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/);
this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.7.0] — 2026-05-26

### Added

#### Consent inventory + consent-grant posture

- `internal/inventory/consent/` — new L1 inventory slice. **A consent grant is evidence of delegated access, not identity truth.** Two entities: `consented_application` is the application as it *presented itself* in a consent flow — keyed on the verifiable anchor `(source, client_id)`, with `display_name` / `publisher` / `home_tenant` / `redirect_uris` held as untrusted self-asserted claims beside the one confirmed datum `verified_publisher`; `consent_grant` records that a subject granted an app a set of `scopes` (natural key `(source, external_id)`). Scopes live on the **grant**, not the app — one app receives different scopes from different subjects — and are stored raw; "high risk" is a policy verdict, not a stored fact.
- **Resolution, not promotion**: a resolver may link a presented app to an already-governed identity via `resolved_principal_id` + `resolution_confidence` (`resolved` / `likely_same` / `ambiguous` / `unresolved` / `spoofing_suspected`); `origin` (`first_party` / `third_party` / `unknown`) is **derived** from that resolution, never asserted by the app. An unresolved app is never minted into a principal of its own — it stays a posture signal, and `principals` keeps meaning "an identity we govern".
- `internal/migrations/20260525140000_consent.go` — `consented_application` + `consent_grant` tables (resolution/origin/grant_type CHECKs, `(source, client_id)` / `(source, external_id)` unique keys, resolved-principal + app/subject indexes); widens `findings.target_type` and `evidence_chains.normalized_kind` to admit `consented_application` / `consent_grant`.
- `GET /api/v0/consented-applications[/:id]` and `GET /api/v0/consent-grants[/:id]` — read-only list (app filters: `origin`, `resolution_confidence`, `verified_publisher`, `resolved`, `resolved_principal_id`; grant filters: `grant_type`, `active`, `owned`, `consented_application_id`, `consenting_principal_id`) + lookup-by-id. The `Upsert` write path stays internal to connector/ingest actions.
- `internal/engines/policy_assessment/actions/assess/` — **consent pass** (`runConsentPass`): evaluates app-level posture against each presented app and grant-level posture against each grant, denormalising the owning app's `origin` / `verified_publisher` into the grant facts so a grant policy can reason about both the scope and who received it. Emits the `scope:consent` facet (disjoint from `scope:account` / `scope:workload` / `scope:secret`), reuses the `OwnerTerminusResolver` port to resolve a consenting principal's terminus, and records findings keyed to the app or the grant on the `(target_type, target_id)` axis.
- `cartridges/ispm-consent-grants/` — new cartridge, OPA, namespaces `ispm_consent_grants.application.*` and `.grant.*` (8 policies): display-name collision, unverified reputable publisher; plus grant sensitive-scope-to-unresolved-app, high-risk-consent-to-unverified-publisher, consent-without-owner, consent-owner-terminated, stale consent (Blind Spot via `stack_check` on `last_used_evidence`), and critical app-only privileged scope to a third party.
- `scripts/seed_consent_demo.py` — idempotent demo fabric (uuid5 + ON CONFLICT) seeding both entities against real workloads/humans with a coherent posture story (a resolved first-party app; an unresolved third-party app; a spoofing app impersonating a governed app's name under a foreign anchor; an app claiming an unverified reputable publisher; plus grants covering sensitive scopes, consent by a terminated human, an app-only privileged grant with no last-use Blind Spot, and an orphaned delegated grant).

#### Secret inventory + credential posture

- `internal/inventory/secrets/` — new L1 inventory slice owning two secret entities, split by shape. **Secrets are authentication evidence, not identities.** `secret_plain` holds opaque material (`password` / `connstring` / `token` / `api_key`) with token `scopes` and a value `fingerprint`; `secret_certificate` holds PKI material (`x509` / `openssh`) with `usage[]` and structured PKI columns (subject, issuer, serial, key algorithm/size, `self_signed`, `is_ca`, validity window). Natural key `(source, external_id)` on both.
- A secret is an **edge, not a node**: `found_in_application_id` / `found_in_location` (the holder / client) → `target_application_id` + `account_id` (what it authenticates to / as), with `principal_id` as the owner/subject. All ends are nullable; a `CHECK` forbids a secret with no locus. A token is usually found in its own target app, while a password / connection string / key is found in a client app's config. A system token vs a PAT is read from the linked principal's kind, not stored as a sub-kind.
- `internal/migrations/20260524100000_secrets.go` — `secret_plain` + `secret_certificate` tables (kind/format CHECKs, locus CHECK, `(source, external_id)` unique, locus + principal indexes).
- `GET /api/v0/secrets/plain[/:id]` and `GET /api/v0/secrets/certificates[/:id]` — read-only list (filters: `type`/`format`, `privileged`, `linked`, `target_application_id`, `found_in_application_id`, `principal_id`, `account_id`) + lookup-by-id. The `Upsert` write path stays internal to connector/ingest actions.
- `internal/engines/policy_assessment/actions/assess/` — **secret pass** (`runSecretPass`): lists both secret tables, builds `Facts` with secret facts in `resource.properties` (lifecycle math precomputed in Go), emits the `scope:secret` facet (never `scope:account`/`scope:workload`, so account/workload policies cannot cross-evaluate secrets), dispatches via the shared `evaluateTarget`, and records findings keyed to the secret. New `OwnerTerminusResolver` port resolves a secret owner's terminus (`employment` principal → person-terminated; `workload` principal → lineage) for the owner-terminated check; implemented in the worker composition root.
- `cartridges/ispm-credential-posture/` — new cartridge, OPA, namespaces `ispm_credential_posture.credential.*` and `.certificate.*` (11 policies): long-lived secret, expiring/expired, no-owner linkage, owner-terminated, stale/unverifiable use (Blind Spot via `stack_check` on `last_used_evidence`); plus certificate weak-key, self-signed, expiring/expired, over-long validity, owner-terminated, stale. Findings target the secret on the `(target_type, target_id)` axis.
- `internal/inventory/evidence_chain/` — `normalized_kind` widened to admit `secret_plain` / `secret_certificate` (`internal/migrations/20260525100000_evidence_chain_secret_kinds.go`) so a secret finding's evidence chain can name the secret as its normalized fact.
- `scripts/seed_secrets_demo.py` — idempotent demo fabric (uuid5 + ON CONFLICT) seeding both secret types against real apps/accounts/principals with a coherent posture story (long-lived API key found in a client repo, healthy workload token, PAT owned by a terminated human, connection string with no owner, expired password, reused-fingerprint key; TLS/SAML/SSH certs covering expiring/expired, weak 1024-bit self-signed, over-long validity, owner-terminated, and missing-last-use Blind Spots).

#### Findings polymorphic target (`target_type` / `target_id`)

- `internal/inventory/findings/model.go` — a finding now carries two independent axes: `principal_id` (the **identity** it concerns — an account's owner or a workload's own principal) and `target_type` + `target_id` (the **artifact** the problem sits on: `account` / `workload` / `secret_plain` / `secret_certificate`). This retires the old `account_id` XOR `principal_id` encoding, which overloaded `principal_id` (it held the workload's principal as a target) and left account findings' owner principal unset.
- `internal/migrations/20260524140000_findings_polymorphic_target.go` — adds `target_type` (CHECK) + `target_id`, backfills existing rows (`account` → the account, `workload` → the workload behind its principal), fixes the identity axis for account findings (`principal_id` = the account's owner), drops `account_id`, and recreates the idempotency unique key + "anchor to identity or artifact" CHECK over the new axes. `target_id` is a polymorphic reference (no FK — findings are run-scoped snapshots tracked by `last_seen_run_id`); the identity axis keeps its FK.
- `GET /api/v0/findings?target_type=&target_id=` — findings filter on the new axes.

#### Fixed — data-quality policy scope leak

- The `ispm-data-quality` account-evidence policies (`missing_mfa_evidence`, `missing_owner_evidence`, `missing_last_used_evidence`) carried no `scope:` tag, so their empty tag set was a subset of every facet set and they fired `not_evaluable` (Blind Spot) gaps against the workload and secret populations as well as accounts. Tagged them `scope:account` so they evaluate accounts only; account evaluability is unchanged, the spurious workload/secret gaps are gone.

#### Account-assignment edge + human access-profile projection

- `accounts.principal_id` — new nullable FK (`accounts → principals`, `ON DELETE SET NULL`): the account-assignment edge recording which governed identity holds a mailbox. NULL = unassigned (an orphan account). Mirrors `workloads.owner_employment_id` on the human side. `internal/migrations/20260523140000_accounts_principal_assignment.go` (additive column + partial index).
- `internal/inventory/access_profile/` — new read-only L1 projection slice. Walks `person → employments → employment-principals → accounts → capability_grants`, plus principal-scoped `initiatives`, joining the catalog (capabilities, scope keys, applications) for labels, and folds it into one nested document grouped by application. Owns no table, emits no events. `terminated`/`active`/`expired` are computed at read time.
- `GET /api/v0/persons/{id}/access-profile` — assembled human-access tree (read-only; safe on prefetch/HEAD). 404 on unknown person.
- `GET /api/v0/accounts/{id}` and `GET /api/v0/accounts?application_id=` — read-only account lookup-by-id and paginated list (optionally narrowed to one application) (`internal/inventory/accounts/routes.go` + `Lookup.GetByID` + `Repository.List`). The list also accepts `privileged` / `mfa` / `assigned` boolean filters (cheap posture-count queries — privileged, privileged-without-MFA, unassigned). The account write path (Upsert / Set*State) stays internal to the pipeline; only these reads are exposed. Expose account labels and per-application account posture (privileged / MFA / assignment counts) to read consumers.
- `GET /api/v0/workloads?application_id=` — the workloads list now accepts an `application_id` filter (`Repository.ListByApplication`), so a single application's NHI population can be paginated server-side instead of filtered client-side. Feeds the Application detail page's paginated workloads table.
- `scripts/seed_employee_access_demo.py` — idempotent demo fabric (uuid5 + ON CONFLICT): catalog (capabilities, scope keys, one mapping per capability), employment principals + assigned accounts + capability grants + initiatives (with `valid_until` deadlines) for showcase persons — including terminated employees who still hold privileged accounts (the human-side mirror of the orphaned-workload story).

#### Workload owner-chain resolver + workload_owned_by_terminated finding

- `internal/inventory/workload_lineage/` — new L1 inventory slice: recursive ownership chain resolver (workload → owning employment → person → all person's employments). Classifies terminus as `active_human`, `terminated_human`, `unowned`, or `broken_link`. Append-only `workload_lineage_snapshots` table with `(workload_id, chain_hash)` idempotent unique constraint. Snapshot writes happen exclusively in the assess pass (R1 — GET is read-only).
- `GET /api/v0/workloads/{id}/lineage` — read-only endpoint returning the resolved `OwnershipChain`. No snapshot write on GET.
- `internal/engines/policy_assessment/schemas.go` — additive `Subject *SubjectFacts` field (json `subject`, omitempty) on `Facts`. Nil in the account path; populated only in the workload assessment pass. `SubjectFacts` carries the resolved lineage view; `PrincipalFacts.Owner` remains the raw direct-owner reference — distinct views (F3 contract documented in Go doc).
- `internal/engines/policy_assessment/actions/assess/` — workload assessment pass (`runWorkloadPass`): iterates workloads, resolves ownership chain via injected `LineageResolver` port, writes snapshot via `SnapshotWriter` port, resolves workload → principal, builds `factsForWorkload` (F1: `scope:workload` + `subject:workload`, never `scope:account`), evaluates workload policies via the same dispatcher, records findings keyed to `principal_id`. `targetRef` generalisation preserves account-path `evidence_hash` byte-for-byte (F2).
- `internal/inventory/findings/model.go` — `KindWorkloadOwnedByTerminated = "workload_owned_by_terminated"` constant.
- `internal/inventory/policy_evaluation_outcomes/` — `TargetWorkload = "workload"` target type; DB CHECK is `('account','subject','workload','source','pipeline')`.
- `internal/migrations/20260522100000_workload_lineage_snapshots.go` — additive migration: `workload_lineage_snapshots` table + `(workload_id, chain_hash)` unique constraint + workload_id index.
- `cartridges/ispm-workload-posture/` — new cartridge bundle with `lifecycle/workload_owned_by_terminated` OPA policy. Tags `["assessment","scope:workload","subject:workload"]` — disjoint from account population by construction (F1). Fires when `input.subject.ownership.terminus == "terminated_human"`; `stack_check.requires: ["subject_linkage","ownership_resolved"]` → `not_evaluable` (Blind Spot) when chain is unowned/broken.
- Posture-domain vocabulary is **workload** throughout (resolver, facts, facets, finding kind, PEO target type, cartridge id, rego packages). The core-identity cartridge's `nhi_unowned` policy is renamed to `workload_unowned` to match.

#### Findings current-posture tracking (`last_seen_run_id`)

- `internal/inventory/findings/model.go` — new `last_seen_run_id` column: the most recent run that re-confirmed a finding. `assessment_run_id` stays pinned to first detection; the two diverge once a finding survives a re-run. `Repository.TouchLastSeen` advances it (and `evaluated_at`) on the idempotent-reuse path so a finding stays visible to current-posture views across re-runs. `GET /api/v0/findings?last_seen_run_id=` filters on it.
- `internal/migrations/20260522140000_findings_last_seen_run.go` — adds the column (backfilled from `assessment_run_id`), NOT NULL, indexed.
- `internal/migrations/20260523100000_nhi_to_workload_rename.go` — renames existing data: PEO `target_type` `nhi`→`workload` (+ CHECK), finding kinds `nhi_owned_by_terminated`→`workload_owned_by_terminated` and `nhi_unowned`→`workload_unowned`.

#### Risk-engine and owner-assignment capabilities

- `internal/engines/risk` — minimal factor-decomposed priority scorer.
  Pure, deterministic `Score(Input) Scored` turning finding attributes
  (severity, kind, privilege, MFA, active) into a capped 0..100 score
  plus the named factor contributions that produced it.
- `internal/engines/owner_assignment` — minimal owner resolver. Builds
  an application→owner map from the `applications.owner` column once per
  run; a finding inherits its account's application owner for routing.
- Findings gain triage denormalisation, stamped at assess time so a
  finding is self-describing without joins: `application_id`, `source`,
  `cartridge_ref`, `owner_ref`, `priority_score`, `priority_factors`
  (jsonb), plus `recommended_action` + `remediation` sourced from the
  cartridge finding metadata.
- `applications` gains a nullable `owner` column (inventory data, not
  UI-managed config).
- Findings list API adds `application_id`, `source`, `cartridge`,
  `owner`, and `exclude_kind` filters; results sort by `priority_score`
  DESC then `detected_at` DESC.
- Cartridge manifest parses the `finding` block (`FindingMeta`) and
  `default_recommendation`.

#### Honest canonical ISPM posture

- `policy_assessment` facts envelope adds `target.account_mfa_enabled`
  and three not-yet-fed truth-presence keys (`subject_linkage`,
  `activity_telemetry`, `initiative_state`), all false, so posture
  policies that need relational/temporal truth surface as honest
  `not_evaluable` blind spots instead of silently passing.

### Fixed

#### `ispm-core-identity-posture` policies now evaluate honestly

- `privileged_access` no longer falls back to `null` (silent
  `not_matched`) on an account-level match: its decision object read the
  optional `input.action` / `input.target.privilege_level` directly, and
  an undefined reference undefined the whole rule. Now read via
  `object.get`. Regression-locked in the OPA mechanism test.
- `mfa_less_privileged` reads observed MFA from `target.account_mfa_enabled`
  (was the never-populated `input.subject.mfa_enabled`) and is gated by a
  `stack_check` on `mfa_evidence`.
- The six data-dependent core policies (orphaned, terminated, dormant,
  unused, drift, nhi) declare `stack_check` + scoping `tags`, so they
  emit honest `not_evaluable` blind spots rather than silent passes.

## [0.6.0] — 2026-05-17

### Added

#### `engines/inventory_normalize.person` action (`internal/engines/inventory_normalize/actions/person/`)

- New minimal normalize action that reads the lake batch, pulls
  `external_id` + `payload.full_name`, upserts the `persons` table
  directly via `ctx.Tx`. Wires `dataset_type=person` end-to-end
  through `inventory_import` (added to the dataset whitelist) so
  a synchronous CSV import can hit it in one request.
- Registered in `cmd/backplane/main.go` alongside the other
  normalize actions.

#### `engines/inventory_import` — synchronous CSV-import façade (`internal/engines/inventory_import/`)

- New endpoint `POST /api/v0/inventory/import`. Body:
  `{source, dataset_type, correlation_id?, records[]}`. Response:
  `{ingest: {…verbatim ingest counters…}, normalize: {…action result…}}`.
  Status codes: 200 on success, 400 on envelope validation
  failures, 500 otherwise.
- Pipeline inside one request:
  1. `inventory_ingest.Process` with `SkipEvent=true` — writes
     lake + audit row, MQ trigger is suppressed so the async
     pipeline does not race against the same batch.
  2. Look up the normalize action for `dataset_type` against the
     internal whitelist (`employee`, `account`, `orgunit`,
     `access_grant_record`). Unknown values rejected.
  3. Dispatch the action via `actionReg.Dispatch` inside a single
     `bun.DB.RunInTx`. Action failure rolls back every PG write.
- `inventory_ingest.Request` extended with `SkipEvent bool`. Default
  `false` keeps existing callers (HTTP `/ingest`, MQ consumer)
  emitting the event as before; `inventory_import` is the only
  caller that flips it true.
- Wired into `cmd/backplane/main.go` alongside the other
  RegisterRoutes calls. Async path (`/ingest` + MQ + pipelines)
  remains untouched.

#### `engines/access_generate.run` action + wiring (`internal/engines/access_generate/actions/run/`, `cmd/backplane/main.go`)

- New orchestrator action `access_generate.run` — thin wrapper that
  parses `principal_id` + optional `application_id` / `capability_id`
  from pipeline args, calls `Engine.Recompute`, returns counters.
  Pipeline YAML:
  ```yaml
  steps:
    - name: regenerate
      engine: access_generate
      action: run
      args:
        principal_id: "{{ event.principal_id }}"
  ```
- Engine wired in `cmd/backplane/main.go`: constructs the
  `capabilities.Repository` (new — see below) and the new
  `initiatives` repository, then builds `access_generate.Engine`
  against the existing principals / employments / org_units /
  applications repositories and registers the action in the
  orchestrator registry.
- `Engine.EventSink` aliased to `events.Sink` so the composition
  root passes its real sink in directly — no adapter shim.
  `Recompute` stamps a single `correlation_id` on every envelope it
  publishes in one pass, so consumers can tie all creates / tombstones
  from one run together.

#### `inventory/capabilities` — read-side repository (`internal/inventory/capabilities/repository.go`)

- `Repository` interface with `GetByID(ctx, uuid)` and
  `GetBySlug(ctx, string)`. `BunRepository` implementation;
  `ErrNotFound` returned for missing rows. Catalog mutation surfaces
  stay where the catalog import flow lives — this slice exposes
  read paths only.

#### `engines/access_generate` — generative observer (`internal/engines/access_generate/`)

- `inheritance.go` fully wired: Principal (kind=employment) →
  Employment (IsActiveAt now) → OrgUnit → walk parent chain →
  distinguished name (`corp/europe/engineering`). Match
  `rule.SourceOrgUnitDN` exactly (prefix-match TBD).
- `expandGrant` resolves `application_slug` via
  `applications.GetByCode` and optional `capability_slug` via
  `capabilities.GetBySlug`; populates `Justification` with
  `source_rule_id`, `source_org_unit_dn`, and `capability_slug`
  when present.
- `buildOrgUnitDN` walks up to 64 levels deep (safety cap), joins
  names with `/`.
- Pure-function unit tests: 8 for `diff` (covering empty inputs,
  all-new, all-removed, match-noop, same-target-different-rules,
  account-vs-grant distinct keys, partial overlap, empty
  source_rule_id matching) + 6 for `ParseInheritanceRule` (happy,
  empty body, missing dn, no grants, missing application_slug,
  malformed types).

#### `engines/access_generate` — generative observer skeleton (`internal/engines/access_generate/`)

- Single entry point `Engine.Recompute(ctx, principalID, RecomputeFilter)`.
  Every trigger (orchestrator pipeline action, beat-scheduled pass,
  ad-hoc REST call) reduces to a Recompute call. `RecomputeFilter`
  narrows scope by application_id / capability_id; empty filter
  rebuilds the whole (principal, ∀ apps, ∀ capabilities) scope.
- Three sources fan into a single `[]plannedInitiative`:
  - `inheritance` — cartridge rules (`Mechanism == "inheritance"`)
    against the principal's Employment + OrgUnit. Rule loading via
    `cartridges.Provider.Policies(BundleRef)` — never walks the
    filesystem directly. Rule body shape:
    `{source_org_unit_dn, grants: [{application_slug, capability_slug?}]}`.
    Skeleton stops short of DN-walking + slug → id resolution —
    marked TODO in `inheritance.go`.
  - `requested` — stub with detailed contract comments. Reads
    approved ITSM requests once the ITSM Gateway lands; returns nil
    today.
  - `delegated` — stub, same shape. Reads active delegations from the
    ITSM Gateway when ready; returns nil today.
- Diff key `(kind, application_id, capability_id, source_rule_id)`;
  `source_rule_id` travels in `Justification`. Two rules accidentally
  pointing at the same target produce two separate initiatives —
  matches the user's "access ⇐ any single active justification".
- Transactional: collect → filter → load current → diff → Create +
  Tombstone all run inside one `bun.DB.RunInTx`. MQ events
  (`inventory.initiative.created`, `inventory.initiative.tombstoned`)
  are staged inside the tx and published after commit so a broker
  outage cannot leave subscribers ahead of the DB.
- Topic constants `TopicInitiativeCreated`,
  `TopicInitiativeTombstoned` exported for the future `access_validate`
  and `access_promote` engines to subscribe by symbol.

#### `inventory/initiatives` — desired-state audit slice (`internal/inventory/initiatives/`, migration `20260513100000`)

- New `initiatives` table holds the justification behind every
  desired-state decision. Target shape: `capability_id IS NULL` →
  account-initiative ("principal needs an account in this
  application"); `capability_id IS NOT NULL` → grant-initiative
  ("principal needs this capability").
- Multiple active initiatives per target are normal — access ⇐ any
  single active justification. No partial unique index on the
  active set.
- Audit-only: rows are **never** deleted. Closure is expressed by
  stamping `tombstoned_at`. No `closed_by`, no `closure_reason` —
  the source of the justification is revoked in its own system
  (org-unit transfer, request cancellation), the platform reacts
  by tombstoning the initiative.
- Repository (`internal/inventory/initiatives/repository.go`):
  `Create` / `Tombstone` / `GetByID` / `List`. No `Delete` method
  by design. `Tombstone` is idempotent — repeat calls on an
  already-tombstoned row return nil; `ErrNotFound` only surfaces
  when the id does not exist.
- `ListFilter` supports filters on principal / application /
  capability plus mutually-exclusive flags `AccountInits /
  GrantInits` and `ActiveOnly / TombstonedOnly`, plus `Kind`.
- Kind values closed list: `KindInheritance` (from org-unit today,
  project later), `KindRequested` (approval workflow), `KindDelegated`.
  Grace-period extensions are not a separate kind — they are
  expressed as a follow-up initiative with a bounded `valid_until`
  or as `valid_until` being pushed out on an existing row.
- `valid_from` and `valid_until` columns carry the planned validity
  window. `valid_from` defaults to NOW() on insert (column default
  + Go fallback in `Repository.Create`); `valid_until` NULL means
  open-ended. `Initiative.IsActiveAt(t)` / `IsActive()` combine the
  window check with the tombstone check; `ListFilter.ActiveOnly`
  does the same in SQL.

#### `inventory/accounts` — three-column state machine (`internal/inventory/accounts/`, migration `20260513090000`)

- New columns `desired_state`, `validated_state`, `effective_state` on
  the `accounts` table. Vocabulary: `not_exist / pending / blocked /
  invited / active`, enforced by per-column CHECK constraints.
  Defaults to `pending` so new rows always satisfy the constraint.
- Backfill: existing rows derive `effective_state` from `is_active`
  (true → `active`, false → `blocked`); `desired_state` and
  `validated_state` start as `pending` so generative and the PDP
  validator assess each row on the next pass.
- Indexes: single-column index on each state column plus the
  composite `ix_accounts_state_divergence (application_id,
  validated_state, effective_state)` for the access_apply working-set
  scan (`validated_state <> effective_state`).
- Repository: single-writer setters `SetDesiredState`,
  `SetValidatedState`, `SetEffectiveState` so each column has a
  clearly-owned writer. `Upsert` writes `effective_state` on
  conflict (connector data tells us what *is*) but leaves the other
  two columns untouched.
- `ListFilter` extended with `DesiredState`, `ValidatedState`,
  `EffectiveState` filters plus a `NeedsApply` shortcut for
  `validated_state <> effective_state`.
- `inventory_normalize.account` now derives `EffectiveState` from
  the connector-supplied `status` / `is_active` payload at write
  time, so freshly ingested accounts land with a correct
  effective state instead of the `pending` default.
- Canonical state constants exported as `accounts.StateNotExist /
  StatePending / StateBlocked / StateInvited / StateActive`.

#### App cartridge HTTP surface (`internal/core/cartridges/routes.go`, `internal/core/descriptor/routes.go`)

- `GET /api/v0/cartridges/{id}/apps` — list app cartridges in the
  bundle. Each entry carries id, name, version, target connector,
  states count, descriptor fields count.
- `GET /api/v0/cartridges/{id}/apps/{app_id}` — full app cartridge
  (manifest + account state machine + descriptor recipe). Server-local
  `BasePath` is suppressed in JSON output.
- `POST /api/v0/cartridges/{id}/apps/{app_id}/descriptor` — render the
  descriptor for a (principal, target_state) pair. Request body lets
  callers overlay manifest config values via `application`; keys not
  overridden fall through to the cartridge defaults. Status codes:
  200 success / 400 missing target_state / 404 unknown bundle or app
  / 422 render failure (template, transform, resolver) / 500 compile
  failure (cycle, undeclared ref, unknown transform).
- The render endpoint lives in `core/descriptor` to keep the
  dependency direction one-way (`descriptor → cartridges`).
- Wired into `cmd/backplane/main.go` alongside the existing
  `cartridges.RegisterRoutes`.

#### `core/descriptor` — app descriptor renderer (`internal/core/descriptor/`)

- `Renderer` compiles an `AppCartridge.Descriptor` recipe once and
  renders the per-state descriptor for as many (principal, target
  state) pairs as needed. Safe for concurrent use after construction.
- Bindings exposed to every template: `.Principal` (arbitrary
  inventory record), `.Application` (typically `manifest.Config`),
  `.Descriptor` (cross-field references to already-rendered fields).
  Templates are strict — `missingkey=error` makes a reference to an
  undefined key fail loudly instead of producing `<no value>`.
- Post-template transforms pipeline: `lower`, `upper`,
  `remove_diacritics` (NFD + drop combining marks), `truncate:N`
  (rune-aware, never splits multibyte sequences). Same `lower` /
  `upper` are also reachable from inside templates via FuncMap.
- `ResolverRegistry` resolves `on_collision`. The default
  `StubResolver` returns the value unchanged so the renderer is
  exercisable end-to-end before the database-backed collision
  handlers are wired. Unknown resolver names fall back to the stub.
- Cross-field references (`{{ .Descriptor.<name> }}`) are extracted
  at compile time and topologically sorted; cycles and references
  to undeclared fields are rejected by `NewRenderer`, not at render
  time. Sort is deterministic (alphabetical tiebreak).
- `by_state` recipes with no entry for the target state are omitted
  from the result entirely (rather than set to nil). YAML scalar
  values inside `by_state` (e.g. `userAccountControl.active: 512`)
  pass through with their original type — no template execution, no
  transforms.

#### `core/cartridges` — app cartridge loader (`internal/core/cartridges/apps.go`)

- New `apps/` subdir convention inside a bundle:
  `<bundle>/apps/<app_id>/{manifest.yaml,account.yaml,descriptor.yaml}`.
  Each directory describes one integrated application — identity
  (`manifest.yaml`), account state machine (`account.yaml`), and
  per-state descriptor recipe (`descriptor.yaml`).
- `AppCartridge`, `AppManifest`, `AccountStateMachine`,
  `AccountTransition`, `Descriptor`, `DescriptorField` typed
  projections of the three YAML files.
- `Provider.Apps(ref) (map[string]AppCartridge, error)` lists every
  app cartridge in the bundle. `FilesystemProvider` implements it
  with a flat scan of `apps/`, hidden entries skipped, missing
  subdir tolerated.
- Cross-file validator: `manifest.id` matches directory name,
  `manifest.connector` non-empty, `account.initial_state` is one of
  `states`, every `transition` references known states, every
  descriptor field is exclusively template-shaped or by_state-shaped,
  every `by_state` key references a known state. Failures wrap
  `ErrInvalidApp`.
- Initial app cartridge lives in the existing `popular` bundle at
  `<cartridges-root>/popular/apps/microsoft_ad/` (Microsoft Active
  Directory). Five-state account machine, descriptor for
  `userPrincipalName` / `samAccountName` / `mail` / `displayName` /
  `ou` / `distinguishedName` / `userAccountControl`, including
  `username_numeric_suffix` collision resolver and by-state OU +
  `userAccountControl` bitmask.
- README files explaining the `apps/` layer convention plus the
  per-app README live alongside the cartridge content in the
  cartridges repo, not inside the backplane subrepo.

#### `engines/policy_assessment/mechanisms/sod` — Segregation-of-Duties handler (`internal/engines/policy_assessment/mechanisms/sod/`)

- Detects toxic combinations of capabilities held by a single
  principal. Rule body shape (parsed from `Manifest.Body`):
  `{"conditions": [{"capability_slugs": [...], "min_count": N},
  ...]}`. Every condition must satisfy its `min_count` against the
  principal's `CapabilitySlugs` set for the rule to fire — partial
  matches do not surface a finding.
- `Prepare` JSON-roundtrips the body through a typed `[]condition`
  slice, rejecting empty `conditions`, empty `capability_slugs`, or
  `min_count <= 0` so the catalogue never ships a rule that can
  never fire. Parsed conditions are cached per
  `<cartridge>/<rule_id>`.
- `Evaluate` reads `req.Facts.Principal.CapabilitySlugs`, intersects
  each condition deterministically, and emits the Decision when
  every condition meets `min_count`. Output shape:
  - `Decision.Effect` empty (this is an anomaly, not a gate).
  - `Decision.RiskLevel` = `"high"` for now; future revisions may
    scale by privilege level of matched capabilities.
  - `Decision.Signals` polymorphic — `"sod_conflict"` string code +
    a structured dict `{"kind": "sod_conflict", "principal": ...,
    "conditions": [{"required": [...], "min_count": N, "matched":
    [...]}, ...]}`.
  - `Decision.Reasons` carries `rule_id`, `rule_kind: "anomaly"`,
    `matched_conditions`, `fact_values`
    (`principal.capability_slugs`), and `produced.matched_per_condition`.
- `scope_mode` (`global` / `per_application` / `by_scope_key`) is
  not honoured in this revision — the engine input does not carry
  per-grant scope context yet. Conditions evaluate over the full
  CapabilitySlugs set as if `scope_mode: global`.
- 5 unit tests: all conditions met fires with polymorphic signal +
  structured payload, single-condition shortfall does not fire,
  `min_count > 1` requires that many matches, nil/empty principal
  is graceful (no panic, no fire), Prepare rejects empty / malformed
  bodies.
- Wired in `cmd/worker/main.go` alongside `opa` and `cedar` handlers
  via `sodmech.New()` → `policyDispatcher.Register(...)`. The SoD
  handler is goroutine-safe and prepared once per snapshot reload
  by the worker's `PrepareAll` pass.

#### `engines/policy_assessment/actions/assess` — policy-assessment action + first end-to-end demo cartridges (`internal/engines/policy_assessment/actions/assess/`, `cartridges/popular/`)

- `policy_assessment.assess` action — orchestrator-registrable unit
  of work. One invocation = one assessment run row. The action walks
  the active accounts population (`accounts.Repository.List`, new
  method exposing a paginated snapshot keyed off `bun.IDB`), builds
  `Facts` per account, dispatches every applicable policy through
  the engine, and writes one finding row per matched policy.
- Args: `triggered_by`, optional `application_id` scope narrowing,
  `mechanisms` allowlist, `created_by` audit field. Result:
  `assessment_run_id` plus counters
  (`accounts_evaluated`/`policies_applied`/`matched`/
  `findings_created`/`findings_reused`).
- Idempotency: `evidence_hash` is a stable SHA-256 of
  `(cartridge_ref, rule_id, account_id, first_string_signal)`.
  A re-run that produces the same finding hits the DB unique
  constraint on the evidence tuple; the action catches the
  duplicate-key signal (PG SQLSTATE 23505 / constraint name
  `uq_findings_evidence`) and increments `findings_reused` instead
  of failing.
- Finding kind defaults to the first string entry in
  `Decision.Signals` — the kernel convention — falling back to
  manifest rule_id, then "anomaly". Severity prefers the manifest's
  static value and falls back to `Decision.RiskLevel`. Account
  anchor populates `account_id`; principal anchor stays nil for now
  (account-only assessments).
- Worker composition root (`cmd/worker/main.go`) wires the engine:
  `policy_assessment.Store` boot-loaded from the cartridges
  provider, dispatcher with `opa` + `cedar` handlers registered,
  `PrepareAll` over the snapshot, then `assess.Register` injecting
  store + dispatcher + repos for accounts, assessment_runs, and
  findings.
- First end-to-end demo cartridges (`cartridges/popular/`):
  - `pipelines/policy_assessment.yaml` — one-step pipeline,
    schedule trigger `every: 1h`, calls
    `policy_assessment.assess` with passthrough args.
  - `policies/access_risk/privileged_accounts.{meta.json,rego}` —
    OPA-mechanism cartridge. Surfaces every account flagged
    `account_is_privileged=true` as a finding with `risk_level:
    medium`, `signals: ["privileged_account"]`, reasoned audit
    payload. Tags `["assessment", "scope:account",
    "resource:Account", "account:privileged"]` line up with the
    facets the action emits per account.

#### `engines/policy_assessment/mechanisms/opa` — Rego predicate evaluator (`internal/engines/policy_assessment/mechanisms/opa/`)

- Embedded OPA (`github.com/open-policy-agent/opa@v1.17.0`) is back in
  the dependency tree, in-process eval via `rego.PreparedEvalQuery`,
  no sidecar. Cedar stays for AuthZ gates; OPA covers anomaly findings
  + generative rules (orphan accounts, terminated access, birthright
  joiner, leaver grace).
- `Handler.Prepare` reads the sibling `.rego` file (default name =
  manifest basename with `.rego`; override via `body.policy_file`),
  parses the module via `ast.ParseModule` to extract the package path,
  and compiles a `PreparedEvalQuery` targeting `data.<package>`. The
  cache is keyed by `<cartridge_ref>/<rule_id>` and refreshed on every
  Prepare so a reloaded policy supersedes the previous version.
- `Handler.Evaluate` marshals `Facts` through JSON into the
  snake_case input shape the kernel `RULE_CONTRACT.md` documents
  (`input.principal`, `input.target`, `input.action`,
  `input.context`, `input.threat`, `input.now`, …) and runs the
  prepared query. The result map is split into `decision` and
  `projected_facts`, mapped 1-to-1 to
  `policy_assessment.RuleResult`:
  - `Decision.Effect` / `RiskLevel` / `Reasons` are typed.
  - `Decision.Signals` and `ProjectedFact.Signals` stay polymorphic
    `[]any` — string codes and structured dicts coexist in the same
    list, matching the kernel `Signal = str | dict` union.
  - `ProjectedFact.Target` re-encodes through JSON so snake_case Rego
    output populates the typed `TargetFacts` struct.
  - `ProjectedFact.ValidFrom` / `ValidUntil` parse RFC 3339 strings.
- `Matched` flips true when the policy fires (`Decision != nil` or
  `len(ProjectedFacts) > 0`); a rule whose body did not satisfy
  returns `Matched=false` with a nil Decision and empty
  ProjectedFacts — the dispatcher / caller treats this as "policy not
  applicable", not as a deny.
- `handler_test.go` covers five cases: reactive gate allow (kernel
  RULE_CONTRACT case 2), reactive anomaly with polymorphic signals
  mixing a string code and a structured `{"kind": ..., "extra": ...}`
  dict (kernel case 1), generative birthright with two projected
  facts (kernel case 3), no-match (rule body unsatisfied), and
  explicit `body.policy_file` override.

#### `inventory/findings` + `inventory/policy_assessment_runs` — persistence for policy-assessment output (`internal/inventory/{findings,policy_assessment_runs}/`)

- New migration `20260508100000_findings_and_assessment_runs` creates
  two tables in one transaction with `UUID` primary keys.
- `policy_assessment_runs` — one row per policy-assessment pass.
  Carries `status` (pending / running / completed / failed) and
  `triggered_by` (manual / api / schedule) as `VARCHAR(32)` with
  `CHECK` constraints; scope narrowing via optional
  `scope_principal_id` / `scope_application_id` FKs; counters
  `findings_total`, `findings_by_severity` (`jsonb`),
  `findings_created_count`, `findings_reused_count`; operator
  timestamps `started_at` / `completed_at` / `created_at` with
  terminal-state and pending-state `CHECK`s.
- `findings` — one row per detected violation or anomaly. References
  `policy_assessment_runs(id)` via `assessment_run_id` (required)
  plus optional anchors `principal_id` → `principals(id)`,
  `account_id` → `accounts(id)`, `policy_id` → `policies(id)`,
  `scope_key_id` → `capability_scope_keys(id)`. `kind` is a free
  `VARCHAR(64)` — vocabulary owned by the emitting policy.
  `severity` and `status` are `VARCHAR(32)` with `CHECK` constraints
  (`critical/high/medium/low` and
  `open/acknowledged/resolved/mitigated`).
  `matched_capability_grant_ids`, `matched_effective_grant_ids`,
  `matched_access_fact_ids` are `jsonb` lists. `evidence_hash` is the
  canonical idempotency key, baked into a `UNIQUE NULLS NOT DISTINCT`
  constraint over `(kind, principal_id, account_id, policy_id,
  scope_key_id, scope_value, evidence_hash)`. `active_mitigation_id`
  / `proposed_mitigation_id` are plain `UUID` columns with no FK; the
  constraint lands when the mitigations slice ships. `CHECK
  ck_findings_principal_or_account` enforces at least one anchor.
- Indexes: `ix_findings_principal_status`,
  `ix_findings_policy_status`, `ix_findings_kind_status_detected`,
  `ix_findings_severity_status`, `ix_findings_active_mitigation_id`,
  `ix_findings_proposed_mitigation_id`, `ix_findings_assessment_run_id`.
  Same idea for `policy_assessment_runs`: status, scope,
  `created_at DESC`.
- Both slices follow the standard pattern: `doc.go`, `errors.go`,
  `model.go`, `repository.go` (bun-backed with an in-process mock
  for tests), `routes.go`, `routes_test.go`. Repository interfaces
  expose `GetByID` / `List` (paginated, filtered) plus write methods
  reserved for the future policy-assessment action — `Insert` /
  `Update` on assessment runs, `Insert` on findings.
- Read-only HTTP surface, mounted on `/api/v0`:
  - `GET /policy-assessment-runs` (filters: `status`,
    `triggered_by`, `scope_principal_id`, `scope_application_id`,
    `limit`, `offset`), `GET /policy-assessment-runs/:id`.
  - `GET /findings` (filters: `principal_id`, `account_id`,
    `policy_id`, `assessment_run_id`, `kind`, `status`, `severity`,
    `limit`, `offset`), `GET /findings/:id`.
  - No `POST` / `PATCH` / `DELETE`: findings are written by the
    policy-assessment action (next milestone), status transitions
    will arrive through dedicated action endpoints in their own
    slice.
- Both slices wired in `cmd/backplane/main.go` alongside the existing
  policies and pipelines mirrors.

#### `engines/policy_assessment` — engine + dispatcher + Cedar mechanism + kernel `RULE_CONTRACT` mirror (`internal/engines/policy_assessment/`)

- Single engine, one dispatcher, N mechanism handlers. Callers
  (`cmd/pdp` AuthZen transport, future policy-assessment action in `cmd/worker`)
  build a `policy_assessment.Request` (carrying typed `Facts`) and
  dispatch by Mechanism. Handlers return a uniform `Output` carrying
  a `RuleResult` (kernel-canonical: `Decision` and/or
  `ProjectedFacts`) plus engine-side diagnostics (Matched + Signals +
  Evidence + Confidence + Payload); aggregation rules belong to the
  caller, not the engine.
- `schemas.go` mirrors the kernel rule contract
  (`aurelion-kernel/src/engines/policy_assessment/RULE_CONTRACT.md`)
  1-to-1: `RuleResult{Decision?, ProjectedFacts[]}`,
  `Decision{Effect, RiskLevel, Signals, Reasons}`,
  `ProjectedFact{Target, Initiative, ValidFrom, ValidUntil,
  DesiredState, RiskLevel, Signals, Reasons}`, `Reason`, `Facts`
  with typed sub-sections (`PrincipalFacts`, `TargetFacts`,
  `ContextFacts`, `PrincipalContextFacts`, `ThreatFacts`,
  `OwnerFacts`, `InitiativeFact`). Go-side adds two transport
  extensions for Cedar / REST AuthZ: `Resource{Type, ID, Properties}`
  and `EntityRecord{UID, Attrs, Parents}` — these have no kernel
  counterpart because the kernel never had a Cedar mechanism.
- Rule-level `Signals` is **polymorphic `[]any`** — each entry is
  either a plain string code (`"orphaned_account_recent_login"`) or
  a structured dict (`{"kind": "sod_conflict", ...}`) — mirroring
  the kernel `Signal = str | dict[str, Any]` union. Engine-side
  Output exposes a typed `AssessmentSignal{Code, Severity, Message,
  Payload}` for dispatcher telemetry; it is **not** part of the
  rule contract.
- Terminology: the actor is `Principal` everywhere in the Aurelion
  contract (`Facts.Principal`, `PrincipalFacts`,
  `PrincipalContextFacts`, `TargetFacts.PrincipalID`). AuthZen wire
  format calls it `subject`; the transport layer translates it in
  `buildFacts`.
- `Store` is the in-memory catalogue — `Reload(provider)` rebuilds
  via `cartridges.Provider`, swapped via `atomic.Pointer`. Reads
  (`All`, `SelectByFacets`, `SelectByMechanism`) are lock-free.
  `RunStoreWatcher` polls `.meta.json` / `.cedar` / `.prompt`
  mtimes (default 5 s) and triggers reload on diff; failures leave
  the previous snapshot in effect.
- **Tag-based pre-filter** is the contract for coarse policy
  selection. Manifest grows a `tags []string` field; the request
  caller derives "facets" (action / resource type / principal type +
  caller-supplied context); `SelectByFacets` keeps every entry whose
  `Manifest.Tags` are a subset of facets. An untagged policy matches
  every request by default. Implemented as a column on the PG
  `policies` mirror too (`text[]` + GIN index, migration
  `20260508090000_policies_tags`).
- `mechanisms/cedar` handler — backed by `github.com/cedar-policy/cedar-go`.
  Reads a sibling `.cedar` text file (default name = manifest
  basename, override via `body.policy_file`), compiles a Cedar
  PolicySet at Prepare-time, runs `IsAuthorized` per Evaluate.
  Result mapping is semantics-correct:
  - Allow + Reasons ≠ ∅ → effect=allow.
  - Deny + Reasons ≠ ∅ → effect=deny (a forbid policy fired).
  - Deny + Reasons = ∅ → "not applicable" (no Decision emitted);
    the aggregator does not treat this as a verdict.
- Cedar handler reads from `Request.Facts`: `Principal` (auto-built
  as Cedar entity with attrs `is_active = (Status == "active")`,
  `mfa_enabled`, `tenant_id`, `email_verified`, …), `Action` →
  `Action::"<name>"` UID, `Resource` (falls back to
  `Target.ResourceType+Resource`), `Context` + `Threat` flattened
  into the Cedar Context Record, `Entities` mounted as graph for
  ABAC / ReBAC `in`-checks. Diagnostic reasons map to
  `Reason{RuleID, RuleKind, Produced: {cedar_policy_id, position}}`.

#### `cmd/pdp` — AuthZen 1.0 transport + Cedar wiring

- `cmd/pdp/transport/authzen` — thin HTTP adapter:
  1. Parse the AuthZen request envelope.
  2. Derive facets — `["authz", "action:<name>", "resource:<type>",
     "principal:<subject.type>"]` plus flattened `context.*` (nested
     map values join with `:`). AuthZen wire calls the actor
     `subject`; the facet is normalized to `principal:` to match the
     Aurelion contract.
  3. `Store.SelectByFacets(facets)` — coarse pre-filter; tags ⊆
     facets ⇒ candidate.
  4. `buildFacts(req)` translates the AuthZen envelope into a
     `policy_assessment.Facts`: `req.Subject` → `Facts.Principal`
     (with typed mapping for `status` / `org_unit` / `tenant_id` /
     `mfa_enabled` / `email_verified`, the rest into
     `Principal.Attributes`); `context.entities` → `Facts.Entities`;
     `context.transport` / `country` / `ip` → `ContextFacts`.
  5. Dispatch each candidate through the engine dispatcher.
  6. Aggregate per AuthZen rules: deny-wins; ≥1 allow with no deny
     → allow; otherwise default deny. Obligations are not part of
     the kernel rule contract; `Response.Context.Obligations` stays
     in the AuthZen wire shape but is left empty (transports may
     populate it by extracting structured signals from
     `Decision.Signals` matching `{"kind": "obligation", ...}`).
  7. Marshal `Response` with `decision` + `context.{reasons,
     obligations, rules_count}`.
- `cmd/pdp/main.go` wired through: dispatcher, `Store` boot-load +
  watcher goroutine, prepare poller (re-runs `Prepare` against the
  current snapshot every tick), AuthZen transport mounted on
  `POST /access/v1/evaluation`. `GET /healthz` reports
  `policies_total`, `policies_per_mech`, `handlers`.

#### `policies` + `pipelines` PG mirror with cartridge sync (`internal/inventory/{policies,pipelines}/`, `internal/core/{policies,pipelines}/`)

- New inventory tables `policies` and `pipelines` mirror the
  current set of cartridge-defined rules and pipeline
  definitions. Cartridges remain the source of truth; the
  tables are projections rebuilt by the backplane sync loop
  so callers (Studio, future REST query surfaces) can
  reference rules / pipelines by stable id without walking
  the cartridge tree. Rego bodies and pipeline YAMLs are
  NOT mirrored — only metadata.
- Natural key `(cartridge_ref, rule_id)` for policies and
  `(cartridge_ref, name)` for pipelines. Mechanism is a
  free-form `VARCHAR(64)` that names a class-of-evaluation
  handler (cedar / sod / risk_scoring / llm_classification /
  graph_analysis / …); no enum on the platform layer.
- Soft-delete semantics: when a cartridge stops shipping a
  rule / pipeline, the sync loop sets `is_active=false` and
  stamps `removed_at`. The row stays — findings already in
  flight can still reference it. Bringing the rule back
  resurrects the same id in place via
  `ON CONFLICT ... DO UPDATE`.
- `core/policies.Manager` and `core/pipelines.Manager` own
  the reconciliation passes; each `RunSyncLoop(ctx, db,
  interval)` wraps every tick in a `pg_try_advisory_lock`
  so N backplane replicas can all tick on schedule and only
  one reconciles — same pattern as
  `orchestrator/beat`. Default cadence: 5 s.
- Read-only HTTP surface for both projections:
  - `GET /api/v0/policies` (filters: `cartridge_ref`,
    `mechanism`, `include_inactive`, `limit`, `offset`);
    `GET /api/v0/policies/:id`.
  - `GET /api/v0/pipelines` (filters: `cartridge_ref`,
    `include_inactive`, `limit`, `offset`);
    `GET /api/v0/pipelines/:id`.
  - No `POST` / `PATCH` / `DELETE`: every change lands via
    a cartridge edit picked up by the next sync tick.

#### `core/cartridges/watcher.go` — mtime-based change detector (`internal/core/cartridges/`)

- New `cartridges.Watcher` polls the cartridges root and
  reports when any tracked file's mtime, presence, or
  identity differs from the previous scan. `Run(ctx,
  interval, onChange)` seeds the state on the first tick
  (no reload) and invokes the callback on every subsequent
  diff. Failures are logged; the previous catalogue stays
  in effect. Suffix filter (`.rego`, `.meta.json`,
  `.yaml`, …) lets each consumer narrow what counts as a
  "change" for them. Default cadence: 5 s.
- Per-process reload paths wired up:
  - `orchestrator.RunCatalogWatcher` rebuilds the pipeline
    catalog in `cmd/worker` and `cmd/backplane` when
    cartridge YAML changes. The catalog itself gains
    `Catalog.Reload(provider, loader, ids)` plus internal
    `sync.RWMutex` so Get / All / Sources stay
    goroutine-safe across the swap.
  - Future mechanism-hosting processes (Cedar PDP, scan
    workers) wire their own watchers using the same helper.
- No MQ events on cartridge changes — mtime polling at 5 s
  is enough for the design baseline. PG mirror sync and
  per-process catalog reload tick independently; no
  coordination needed.

#### `engines/policy_assessment` — single engine, dispatched per mechanism (`internal/engines/policy_assessment/`)

- One engine, one dispatcher, many mechanisms — each mechanism owns
  one class-of-evaluation problem end-to-end (manifest body schema,
  backing infrastructure, native result → `policy_assessment.Output`
  translation, where Output carries a `RuleResult`).
- Mechanism index — every entry is a README contract; no Go
  implementation yet:
  - `cedar` — Cedar policies for AuthZ (RBAC + ABAC + ReBAC
    in one language). Author writes Cedar text in a sibling
    `.cedar` file; backend is `cedar-go`.
  - `sod` — Segregation of Duties; DB-backed combinatorial
    evaluator, pure Go.
  - `risk_scoring` — Signal collectors + weighted
    aggregation + threshold tiers.
  - `behavioral` — Baseline / anomaly score against a
    sliding window.
  - `llm_classification` — Prompt + retrieval + structured
    LLM response (via `core/llm`).
  - `graph_analysis` — Toxic-path / cycle / closure
    traversal.
  - `compliance_scorecard`, `quorum`, `windowed_threshold`
    — placeholders.
- Cartridge `Manifest` simplified to a mechanism-neutral
  shape: `rule_id`, `version`, `name`, `mechanism`,
  `severity`, `owner_team`, plus an open-ended `body`
  (`map[string]any`) carrying mechanism-specific fields
  (`policy_file` for cedar, `prompt_template_file` for LLM,
  weights / thresholds for risk_scoring, etc.). Platform
  layer no longer interprets body contents.
- `Manifest.BasePath` carries the absolute path of the
  `.meta.json` itself; mechanism handlers resolve their own
  sibling files (`.cedar`, `.prompt`, …) from it. The
  filesystem provider no longer requires (or knows about)
  any specific sibling extension.
- Sample Rego cartridges `sample_authz/` removed; the
  reference path for AuthZ becomes Cedar text policies once
  the `cedar` handler lands.

### Removed

- **`internal/core/opa/`** — Rego evaluator wrapper around
  `github.com/open-policy-agent/opa/v1/rego`. The
  `policy_assessment` engine targets domain-specific
  mechanisms (cedar / sod / llm / graph / …) rather than a
  generic Rego pipeline; OPA is no longer a dependency.
- **`internal/engines/auth_decisions/`** — the previous
  Rego-only PDP engine. Functionally absorbed by the future
  `cedar` mechanism + an AuthZ runtime caller.
- **`cmd/pdp/` reset to a skeleton.** The binary stays —
  it remains the SLO-isolated host for AuthZ and AuthN.
  All Rego / OPA / auth_decisions imports were stripped; it
  now boots config + PG + RabbitMQ + cartridges and serves
  `GET /healthz`. AuthZen evaluation and mechanism handlers
  land when `cedar` and the other mechanisms are
  implemented.
- **`cartridges/popular/policies/sample_authz/`** — the two
  Rego demo rules. Will be re-shipped as Cedar policies once
  the `cedar` handler lands.
- **`github.com/open-policy-agent/opa`** dropped from
  `go.mod`; `go mod tidy` removed the transitive set.

### Changed

#### Orchestrator-owned action primitives (`internal/core/orchestrator/actions/`)

- Moved `noop` package from `internal/actions/noop` to
  `internal/core/orchestrator/actions/noop`. It is not test-only —
  these are pipeline-shape primitives any cartridge may use.
- Added three new primitives:
  - `noop.fail` — deliberate handler error wrapping `ErrDeliberate`;
    replaces the template-resolver hack in `smoke.fail.yaml`.
  - `noop.constant` — returns an arbitrary JSON object verbatim;
    stubs a producer step before the real action exists.
  - `noop.emit` — publishes a domain envelope through
    `ActionContext.Events`. Falls back to the pipeline run ID for
    `correlation_id`. Non-idempotent: a retried dispatch produces
    a fresh envelope with a new `event_id`.
- `ActionContext` gains `Events events.Sink`. The runner threads it
  through; handlers that don't emit are unaffected. Composition
  root passes the sink at `runner.New`.
- `cmd/worker` now declares the events exchange on the rabbitmq
  connection and constructs an `events.NewMQ` sink — worker
  becomes a first-class producer of domain envelopes alongside
  backplane.
- `smoke.fail.yaml` rewritten to use `noop.fail` honestly instead
  of relying on an unfilled template raising at resolve time.

#### Secret contracts collapsed into the platform package (`internal/platform/secretmanagers/`)

- Removed `internal/core/secret/`. Its `Manager` / `Mutator` /
  `FullManager` interfaces and `ErrNotFound` / `ErrNotImplemented`
  sentinels moved into `internal/platform/secretmanagers/interface.go`.
- Brings secrets in line with the other platform services (`siem`,
  `storage`, `llm`) — every package holds its own contracts +
  factory + one file per backend.
- All consumers (`core/config/*`, every provider in
  `platform/secretmanagers/`, `cmd/backplane/main.go`) updated.
  Direction `core/config → platform/secretmanagers` is the
  legitimate `core → platform` lean; nothing in `platform`
  imports `core` upward.

## [0.5.0] — 2026-04-27

### Added

#### `inventory_normalize` engine — orchestrator-action transforms from lake → typed inventory (`internal/engines/inventory_normalize/`)

- Action-per-dataset model: every supported `dataset_type`
  binds to one Go action registered against the orchestrator
  action registry. Trigger is automatic via the matcher: an
  `inventory.ingest.batch_received` MQ event with
  `dataset_type=X` fires the `inventory.normalize.X` pipeline
  with `batch_id`, `source`, `lake_ref` plumbed in through
  `args_from_payload`. No YAML cartridge schema, no separate
  normalize_runs audit table — orchestrator pipeline_runs
  already records the run lifecycle.
- `account` action: `dataset_type=account` → `accounts`
  upsert keyed by `(application_id, username)`. Carries
  `display_name`, `email`, `is_active`, `is_privileged`,
  `mfa_enabled`, `status`, plus open-ended `attrs` jsonb.
- `employee` action: `dataset_type=employee` → `persons` +
  `employments` + EAV attribute sidecars. One incoming
  record can carry multiple employment periods via
  `payload.employments[]`; each period becomes its own
  Employment row keyed by `(person_id, code, start_date)`.
  `code` derives from `end_date` (`active` if null, else
  `former`). Org-unit linkage resolves a contract-level
  `org_unit_id` first, then falls back to `org_unit_name`
  via display-name lookup; the result is a typed FK on the
  Employment row plus the original name preserved in
  `employment_attributes` when the FK didn't resolve.
- `orgunit` action: `dataset_type=orgunit` → `org_units`
  tree. Walks the contract's `children[]` top-down,
  upserting each node keyed by `external_id` with
  `RETURNING id`, then threads the resolved id down as
  `parent_id` for the next level. Idempotent across
  re-ingests.
- `access_grant_record` action: `dataset_type=access_grant_record`
  → `capability_grants` projection. Per record, resolves
  the account via `(application_id, username)` lookup
  (orphan grants — username with no matching account —
  bump `unresolved_acct` and skip), then walks every
  active `capability_mappings` rule. The projector is a
  pure function: filter by `(application_id, action_slug)`,
  match resource via XOR over
  `(resource_id, resource_kind, resource_path_glob)`,
  resolve scope value, upsert the resulting Grant.
- `resource_external_id` scope-source kind: the projector's
  `resolveScopeValue` gained a fourth discriminator that
  pulls the resource external_id straight off the grant
  record — the common case for "scope IS the thing"
  rules (AD `group_member` → group SID, ACL
  `file_access` → share id, SAP `role_assignment` → role
  code). No lake lookup needed; the value is already on
  the projector input.

#### Inventory model: identity + access pillars

- `persons` slice gains `AttributeLookup` — narrow port
  for cross-app determinator matching (resolves a person
  by `(key, value)` pairs across the `person_attributes`
  EAV sidecar).
- `employments` slice — per-person work-history rows.
  Natural key `(person_id, code, start_date)` plus
  `code='active'` partial unique index that enforces "at
  most one active period per person." `employment_attributes`
  is the EAV sidecar for per-period scalars (`title_id`,
  `title_name`, `org_unit_name` fallback).
- `employment_record_matches` slice — lineage table tying
  one upstream `(source, external_id, period_start_date)`
  triple to one Employment. The `period_start_date` in
  the unique key is what lets the same HRIS record carry
  multiple historical periods without colliding.
- `employee_provider_mappings` slice — per-application
  rules indexing which payload keys feed Person /
  Employment fields. Carries `IsDeterminator` so the
  resolver knows which keys to use for cross-application
  identity resolution vs which to merely upsert as
  attributes.
- `org_units` slice — tree-shaped org structure with
  recursive `parent_id`. Adds `display_name` and
  `is_active` columns. `Lookup` port exposes both stable
  (`GetIDByExternalID`) and fallback (`GetIDByDisplayName`)
  paths for downstream actions.
- `accounts` slice — provider user-mailbox inventory.
  Natural key `(application_id, username)`. `Lookup` port
  fronts the access-projection path. Account → principal
  matching is deliberately deferred to a separate engine.
- `capabilities`, `capability_scope_keys`,
  `capability_mappings`, `capability_grants` slices —
  the access-projection vocabulary and storage. Mappings
  are admin-managed rules (`scope_value_source` is a
  discriminated-union JSONB); grants are the projected
  `(account, capability, scope_key, scope_value)` tuples
  that downstream UIs / detection / certification consume.

#### `inventory_ingest` engine — REST + MQ entrypoint to the data lake (`internal/engines/inventory_ingest/`)

- Atomic responsibility: accept a batch of raw records,
  hash them, anti-join against what the lake already
  knows, write only the changed ones to JSONL, persist a
  metadata row, emit an event. No normalisation, no
  per-entity reasoning. Two transports share the same
  service: REST `POST /api/v0/ingest` for synchronous
  callers with batches in memory, and a separate
  `cmd/ingester` binary that consumes one-record-per-message
  AMQP traffic, windows by `(source, dataset_type,
  correlation_id)`, and calls Process per window.
- `inventory_ingest_batches` table — one row per accepted
  batch with `source`, `dataset_type`, item count,
  `lake_ref`, status (`pending` → `stored` / `failed`),
  per-record counts (`received`, `written`, `skipped`,
  `new`, `changed`), error, `received_at`,
  `completed_at`.
- DuckDB-driven hash anti-join (`internal/platform/storage/
  file_antijoin.go`) reads existing batch JSONL on disk
  and compares SHA-256 of canonical-JSON payloads — same
  external_id with same hash is a no-op, same external_id
  with different hash is a `changed`, fresh external_id
  is `new`.
- Events: `inventory.ingest.batch_received` on success
  (carries `batch_id`, `source`, `dataset_type`,
  `lake_ref`); `inventory.ingest.batch_failed` on lake
  write error. The orchestrator matcher pipes these into
  the dataset-specific normalize pipelines.
- REST surface: `POST /ingest`, `GET /ingest/batches`,
  `GET /ingest/batches/:id`. 50 000-record cap, payload
  stored as `[]map[string]any`, 502 on lake-write
  failure so the client can distinguish bad envelopes
  from unreachable storage.

#### `inventory_discover` engine — pull-side entrypoint for raw inventory data (`internal/engines/inventory_discover/`)

- Active-side counterpart to `inventory_ingest`. Instead of
  waiting for a push, asks a registered connector instance
  to produce a fresh snapshot via the existing AMQP RPC
  channel, then routes the records through the ingest
  engine so the lake-write path stays single-source.
- `inventory_discover_runs` table — one row per Fetch call
  with `connector_instance_id`, `operation`, `dataset_type`,
  lifecycle (`running` → `completed` / `failed`),
  `ingest_batch_id` link, item count, error, timestamps.
- REST surface under `/api/v0/discover/runs`: `POST` to
  trigger a fetch synchronously (envelope validation +
  RPC + ingest in one HTTP roundtrip); `GET` list paginated
  by `started_at DESC`; `GET /:id` for a single run.
  Connector / ingest failures return 502 with the run
  marked failed; envelope errors return 400.
- Reuses the connectors RPCClient unchanged: connectors
  may reply inline or via `result_storage_ref`, and the
  RPC client resolves either shape into a uniform record
  list before discover sees it.
- Events: `inventory.discover.run.started` (on Insert),
  `inventory.discover.run.completed` (on success, includes
  `ingest_batch_id`), `inventory.discover.run.failed` (on
  any failure). Normalize subscribes to
  `inventory.ingest.batch_received`, NOT to these — push
  and pull paths produce one canonical "new batch" event.
- Composition root wires a thin `discoverConnectorAdapter`
  so the engine package has no direct dependency on the
  `integrations/connectors` API surface.

#### Platform / infra

- `internal/transports/ingest_mq` — durable AMQP ingest
  consumer that connects the `cmd/ingester` binary to
  `inventory_ingest.Process`. Per-window aggregation
  (`source`, `dataset_type`, `correlation_id`) batches
  one-record-per-message traffic into single Service
  calls so the lake-write path stays one path.
- `internal/platform/storage/file_antijoin.go` — DuckDB
  scan over batch JSONL with SHA-256 hashing of canonical
  payload JSON. Powers the ingest path's "only write
  changed records" semantic.
- Default lake and SIEM-log paths now live one level
  above each binary's cwd (`../.lake` and
  `../.logs/aurelion.log.jsonl`). Lake and event stream
  are monorepo-wide artifacts, not subrepo-local clutter.
- Six migrations land for the new tables and columns:
  `inventory_ingest`, `inventory_discover`, `accounts`,
  `capability_model` (capabilities + scope_keys +
  mappings + grants), `employee_normalize` (persons +
  employments + matches + provider_mappings),
  `org_units_normalize` (`display_name`, `is_active`).
- `cmd/ingester` — the second runtime binary. Claims
  `AURELION_INGESTER_INSTANCE_ID`, opens N worker
  goroutines (`AURELION_INGESTER_SLOTS`), each consuming
  off the same durable queue with prefetch=1 for fair
  cross-replica distribution.

## [0.4.0] — 2026-04-08

### Added

#### Worker slot registry + `/workers` endpoint (`internal/core/orchestrator/runner/`, `internal/core/orchestrator/routes_workers.go`)

- New `worker_slots` table — one row per live runner slot,
  upserted on slot start, refreshed by a dedicated 5 s
  heartbeat goroutine, deleted on graceful shutdown. The
  `/workers` endpoint reads this registry directly (not
  derived from `pipeline_runs`), so idle slots are visible
  alongside busy ones — derived-from-runs view never showed
  idle workers and that was the gap.
- `WorkerStaleThreshold = 15 s` (3× heartbeat interval)
  filters rows whose heartbeat is older — a crashed process
  disappears within one threshold window.
- Readonly process tags via `AURELION_WORKER_TAGS=gpu,llm,…`
  env var. Worker startup parses the CSV (trim + dedupe),
  shares the same set across every slot of the process, and
  upserts into `worker_slots.tags TEXT[]`. Purely
  informational for now — surfaced in the Studio overview
  panel as chips next to the PID header.
- `GET /api/v0/workers` returns the registry joined with
  per-`worker_id` active-run aggregates (count, earliest
  start, representative current run + pipeline) so the
  Studio overview can render busy/idle status + drill into
  the currently-held run without N+1 lookups.

#### Orchestrator MQ matcher (`internal/core/orchestrator/matcher/`)

- Async RabbitMQ consumer that drives two effects per delivery on
  `aurelion.events`:
  1. Waiter resolution: every wait_for_event step parked in
     `pipeline_event_waiters` whose `match` JSONB is contained in the
     incoming payload (`match <@ payload`) is resolved via the same
     `Service.ResolveEventWaiter` the HITL endpoint uses, so the
     parked run wakes up and the runner re-claims it.
  2. MQ-trigger firing: every pipeline definition with a
     `type: mq` trigger whose `routing_key` matches the delivery AND
     whose `match` predicate is contained in the payload spawns a
     fresh run via `Service.CreateRun(TriggerMQ)`. The partial-UNIQUE
     idempotency index plus `content_hash` dedupe automatically
     squashes RabbitMQ at-least-once redeliveries — re-firing the
     same event for the same in-flight run returns the existing row.
- Effects (1) and (2) run in **independent transactions** so a
  failure in one cannot roll the other back.
- Cluster-wide single-active: matcher holds a session-level
  `pg_advisory_lock` (key `0x4155_5245_4C4D_4154` = "AURELMAT") on a
  dedicated PG connection. Siblings that cannot acquire become warm
  standbys retrying every second.
- New bootstrap settings:
  `RabbitMQ.MatcherQueue` (default `aurelion.orchestrator.matcher`) —
  the durable queue the matcher binds to `aurelion.events` with
  catch-all routing key.

#### Orchestrator Beat (`internal/core/orchestrator/beat/`)

- Periodic scheduler + waiter timeout sweeper, ticking every 10 s.
- Per-tick `pg_try_advisory_lock` (key `0x4155_5245_4C42_4541` =
  "AURELBEA7") so several backplane replicas can run the goroutine
  without double-firing schedules.
- `PreviousFirePoint(now, cron, every)` returns the most recent
  fire-point of a schedule. `every` uses an epoch-anchored floor for
  determinism across restarts; `cron` is parsed by
  `robfig/cron/v3`. `Service.IsScheduleAlreadyFired` dedupes within
  the cron window using a count over `pipeline_runs` filtered by
  `trigger_source=schedule` AND `created_at >= fire_point`.
- Each tick also walks `pipeline_event_waiters` for `expires_at < now`
  and transitions the parked step + run from `awaiting_event` →
  `failed_timeout` via the new `Service.ExpireEventWaiter`
  (`Repository.ScheduleAlreadyFired` is the matching new repo method).
- Launched as a goroutine from `cmd/backplane`; safe to run in every
  replica thanks to the advisory lock.

#### Orchestrator runner + run-mutating REST surface (`internal/core/orchestrator/runner/`, `internal/core/orchestrator/routes_runs.go`, `cmd/worker/`)

- `runner.Runner` is the PG-claim work loop driver. Each instance owns
  one slot inside `cmd/worker`; the loop reclaims stale runs at the
  head of every tick, claims one pending run via the Service's
  `SELECT … FOR UPDATE SKIP LOCKED` + guarded UPDATE, then drives every
  step. Three-transaction-per-step protocol: Tx A claims, Tx B inserts
  the StepRun (committed so failure transactions can see it), Tx C
  runs the action and commits the success transition atomically. On
  action error a fresh Tx D writes step + run failure.
- Heartbeat goroutine refreshes `pipeline_runs.last_heartbeat_at`
  every 3 s and watches for a status flip to `cancelling`; when it
  spots one it cancels the action's context so the handler unwinds
  cleanly, after which the run is marked `cancelled`.
- Resume on re-claim: the runner loads every prior `completed` step
  row before executing the loop and skips steps already done. This
  closes the loop with `wait_for_event` parking — once a waiter is
  resolved, the worker re-claims the run and continues from the next
  step instead of re-running already-done work (which would otherwise
  hit `uq_step_runs_run_step_attempt`).
- Mutating REST surface under `/api/v0`:
  - `POST /pipelines/{name}/runs` — create a run; 201 on fresh insert,
    200 on idempotency dedupe.
  - `GET /pipelines/{name}/runs` — list runs for one pipeline.
  - `GET /pipelines/runs` — global list with `?pipeline=`, `?status=`,
    `?limit=`, `?offset=` filters.
  - `GET /pipelines/runs/{id}` — detail + ordered steps.
  - `GET /pipelines/runs/{id}/steps` — every step attempt for one run.
  - `GET /pipelines/runs/{id}/steps/{step}` — latest attempt for a
    named step.
  - `POST /pipelines/runs/{id}/cancel` — synchronous for pending /
    awaiting_event runs, asynchronous (`cancelling`) for running ones.
  - `POST /pipelines/runs/{id}/retry` — terminal-only.
  - **`POST /pipelines/runs/{id}/steps/{step}/resolve`** — the HITL
    endpoint. An operator passes `{payload: {…}}`; the same
    `Service.ResolveEventWaiter` the matcher will use later marks the
    step complete, deletes the waiter, and re-activates the run.
- `cmd/worker` is now a real runner-bootstrap process: it boots the
  same composition root as `cmd/backplane` (secrets → postgres →
  cartridges → action registry → catalog), spawns
  `AURELION_WORKER_SLOTS` goroutines (default 4), and waits up to 60 s
  for them to drain on SIGINT/SIGTERM.

#### Pipeline discovery + read-only REST surface (`internal/core/orchestrator/`)

- `orchestrator.Catalog` is the immutable in-memory snapshot of every
  pipeline definition discovered at startup. `LoadFromCartridges`
  iterates the configured cartridge ids (or every cartridge the
  provider knows about when the list is empty) and feeds each
  `<cartridge>/pipelines/` directory through the loader. Duplicate
  pipeline names across cartridges fail boot fast.
- `BuildMergedSchema` deep-copies the embedded grammar and injects
  per-action arg / result schemas under
  `$defs.action_args["<engine>.<action>"]` and
  `$defs.action_results[…]`. Merge is purely additive — existing
  schema entries are preserved.
- `BuildActionCatalogue` enumerates every registered action with its
  idempotent flag and both schemas; consumed by `GET /api/v0/actions`.
- New read-only REST surface:
  - `GET /api/v0/pipelines` — sorted summary list.
  - `GET /api/v0/pipelines/{name}` — full definition.
  - `GET /api/v0/actions` — registered action catalogue.
  - `GET /.well-known/pipeline-schema.json` — merged JSON Schema for
    the VSCode YAML completion in aurelion-engineering-studio.
- Composition root wires the catalog after `cartridges.Provider` and
  before `webserver`. Action-ref validation is intentionally OFF
  (`loader.Loader.Actions = nil`) — flip it on once the engine
  packages register their actions.

#### Orchestrator state tables + Service (`internal/core/orchestrator/`)

- Migration `20260530090000_orchestrator` creates the three pipeline
  state tables — `pipeline_runs`, `step_runs`, `pipeline_event_waiters`
  — plus four PG enum types
  (`pipeline_run_status`, `step_run_status`, `pipeline_event_waiter_status`,
  `pipeline_trigger_source`). Partial UNIQUE index
  `uq_pipeline_runs_inflight_idempotency` blocks duplicate in-flight runs
  for the same (pipeline_name, pipeline_version, content_hash) while
  retries (retry_of_run_id NOT NULL) and terminal rows bypass.
- `orchestrator.Service` is the sole writer to all three tables.
  Every status-changing UPDATE WHERE-guards on the expected source
  status; a 0-rowcount triggers refresh-and-branch logic rather than a
  silent retry. The full lifecycle: `CreateRun` with
  partial-UNIQUE dedupe + retry-of bypass, `CreateRetry` (terminal-only
  guard), `ClaimPendingRun` via `SELECT … FOR UPDATE SKIP LOCKED` +
  guarded `pending → running` UPDATE, `RefreshHeartbeat` /
  `ReclaimStaleRun` / `ListStaleRunIDs` (10 s hard-coded threshold),
  the `pending/awaiting_event/running` cancel branches with the
  cancel-vs-complete race silently transitioning to `cancelled`,
  step lifecycle (`CreateStepRun`, `MarkStepSucceeded`,
  `MarkStepFailed`, `MarkStepAwaitingEvent`), and the shared
  `ResolveEventWaiter` consumed by both the matcher and the HITL
  `POST /pipelines/runs/{id}/steps/{step}/resolve` endpoint.
- `Repository` interface keeps Service decoupled from bun for unit
  tests. `BunRepository` ships the production implementation;
  `memRepo` in tests covers idempotency dedupe, retry guards, all
  three cancel branches, complete-vs-cancelling race, waiter resolve
  + run re-activation, and oldest-first claim ordering.

#### Action registry + `noop` actions (`internal/core/orchestrator/registry/`, `internal/actions/noop/`)

- Generic in-memory action registry keyed by `(engine, action)`. Engines
  call `registry.Register[A, R](r, engine, action, idempotent, h)` at
  composition time. Args / result JSON Schemas are derived
  automatically from the handler's input / output struct definitions
  via `invopop/jsonschema`, then compiled with
  `santhosh-tekuri/jsonschema` for runtime validation.
- `Registry.Dispatch` does the full pipeline: args-schema validation →
  JSON-roundtrip into the handler's struct → handler invocation →
  result-schema validation → struct-to-map for storage. The runner
  (Step 6) is the only caller.
- `Registry.Has` implements `loader.ActionLookup` so Step 5's discovery
  step can flip on action-ref validation in the YAML loader.
- `internal/actions/noop` ships two trivial smoke actions —
  `noop.echo` and `noop.sleep` (bounded to 60 s; respects context
  cancellation) — used by the integration test harness and by the
  default smoke pipeline.

#### Pipeline grammar + loader (`internal/core/orchestrator/{grammar,loader}/`)

- Embedded JSON Schema 2020-12 grammar for pipeline YAML
  (`grammar/schema.json`) — the single source of truth for both the
  loader and the well-known endpoint (Step 5). `runs` is reserved as a
  pipeline name to avoid collision with the
  `/api/v0/pipelines/runs/...` route family.
- `loader.Loader` reads YAML, validates against the embedded grammar,
  then runs fail-fast checks: requires order (no forward / self /
  unknown deps), templating (`${args.X}` against declared properties
  and `${steps.S.result.X}` against the transitive requires closure),
  triggers (at most one schedule, mq `args_from_payload` keys +
  values shape).
- `LoadFile` / `LoadDir` / `LoadMany` cover single-file, directory,
  and multi-source loading. Each loaded definition carries an
  immutable `content_hash` = sha256 of canonicalised JSON so the
  runner can detect args drift.
- Optional `ActionLookup` hook (no-op by default; wired to the
  registry on Step 3) lets the loader reject step `(engine, action)`
  refs that aren't registered. Smoke-test passes for every YAML
  currently shipped in `cartridges/journey/pipelines/`.

#### Cartridges provider (`internal/core/cartridges/`)

- Source-agnostic provider for the external cartridges bundle that
  lives outside the backplane git repo (default path: `../cartridges`,
  overridable via `AURELION_CARTRIDGES_ROOT` env or the `cartridges`
  bootstrap secret). A cartridge is a directory containing
  `pipelines/*.yaml` and `policies/<bucket>/<rule>.meta.json` +
  `<rule>.rego` files. The provider only enumerates ids and exposes
  files on disk — it knows nothing about pipeline grammar or rego
  semantics.
- `Provider` interface (`List`, `Materialize`, `Policies`, `Pipelines`)
  plus `Manifest` projection of one `.meta.json` sidecar. `Factory`
  registers named provider constructors mirroring `storage.Factory` /
  `siem.Factory`. `FilesystemProvider` is the only registered source
  today; git / OCI / zip drop in next to it without touching consumers.
- Read-only REST surface under `/api/v0/cartridges`:
  - `GET /cartridges` — list every cartridge id with pipeline / policy
    counts.
  - `GET /cartridges/{id}` — detail with materialized root path.
  - `GET /cartridges/{id}/policies` — list of `Manifest` records.
  - `GET /cartridges/{id}/pipelines` — list of pipeline YAML files
    (filename + absolute path).
- `config.Cartridges` bootstrap section with sane defaults; existing
  deployments without a `cartridges` secret continue to boot.

## [0.3.0] — 2026-03-30

### Added

#### Inventory layer (`internal/inventory/`)

- New `inventory/` layer between `platform/` and `engines/` for the
  core domain entities Aurelion governs. Two foundational shapes:
  - **Employment, not Employee.** A single human can hold several
    concurrent or sequential masks at the same legal entity (e.g.
    full-time developer + part-time QA), each with its own org unit,
    manager, and access posture. Each mask is a row in
    `employments`; the canonical human is a `persons` row. Employment
    state is `Employment.code` — a tenant-defined free-text label
    (`active`, `probation`, `maternity_leave`, `notice_period`,
    `sabbatical`, …) so every company can label their working states
    in their own vocabulary without the platform pretending otherwise.
  - **Principal as the unified identity row.** `Principal` is the
    canonical IAM term and the single point where access decisions
    land: it points at one body (employment / workload / customer) via
    partial FK columns and carries two orthogonal axes — `status`
    (lifecycle posture) and `is_locked` (operational/admin access
    lock). `is_locked` lives ONLY here; no employment / workload /
    customer table carries its own lock column. Locking any kind of
    identity is one and the same operation:
    `POST /principals/:id/lock`.
- `inventory/shared/` — vocabulary constants: `PrincipalKind`
  (`employment`, `workload`, `customer`), kind-specific status enums
  (`WorkloadStatus`, `CustomerStatus`; employment status is free
  text), `CustomerTenantRole`, `CustomerPlanTier`, plus the
  routing-key catalog for every event the inventory layer emits.

#### Slice — persons

- `Person` domain entity with `(id, external_id, full_name)`, unique
  on `external_id`, plus `PersonAttribute` 1→N (cascade) with unique
  `(person_id, key)` — stable per-human attributes (name, primary
  email, phone).
- REST surface under `/api/v0/persons`: list (paginated), create,
  get by id, bulk upsert (cap 500), list/add/remove attributes.
- Events: `inventory.person.{created, bulk_upserted,
  attribute_added, attribute_removed}`.

#### Slice — org_units

- `OrgUnit` self-referencing tree with `external_id`, `name`,
  `parent_id` (SET NULL), `description`, `is_internal`, `created_at`,
  `updated_at`. Internal hierarchy (seeded by deployment) and external
  hierarchies (synced via REST) coexist.
- Service enforces the `is_internal` invariant in code (no SQL
  trigger): API-created nodes are external, internal rows reject
  patch/delete, an external child may not attach to an internal parent.
- REST under `/api/v0/org-units`: list, create, bulk upsert with
  two-pass parent resolution by `parent_external_id`, get, patch,
  delete.
- Events: `inventory.org_unit.{created, updated, deleted,
  bulk_upserted}`.

#### Slice — employments

- `Employment` is a period of work for a single Person: `id`,
  `person_id` (FK CASCADE), `code` (free text, varchar 64),
  `start_date`, `end_date` (nullable = open), `org_unit_id` (SET NULL),
  `description`, timestamps. Partial unique index
  `WHERE end_date IS NULL` for active-employment lookups.
- `EmploymentAttribute` 1→N — period-specific tags (job_title,
  manager_external_id, headcount allocation). Stable per-human
  attributes live on PersonAttribute instead.
- REST surface under `/api/v0/employments`: list, create (with
  arbitrary code), bulk upsert (resolves person/org_unit by
  external_id), get, aggregate patch with `changes`-event, `POST
  /employments/:id/end` (sets end_date and emits ended), list/add/
  remove attributes; plus person-scoped listings under
  `/api/v0/persons/:personID/employments(/active)`.
- Code transitions trigger Principal status recompute via a
  cross-slice adapter; ended employments do the same so a terminated
  mask becomes inactive on its Principal.
- Events: `inventory.employment.{created, updated, ended,
  bulk_upserted, attribute_added, attribute_removed}`.

#### Slice — workloads

- `Workload` is a non-human identity body (service accounts, machine
  identities): id, external_id, name, description,
  `owner_employment_id` (SET NULL — owned by a specific Employment
  mask, not the human, so a workload tied to the developer mask goes
  with the developer mask), `application_id` (SET NULL),
  `WorkloadAttribute`. No is_locked column — that's on Principal.
- REST under `/api/v0/workloads`: list, create, bulk upsert, get,
  patch (no expire endpoint — locking goes through Principal), list/
  add/remove attributes.
- Events: `inventory.workload.{created, updated, bulk_upserted,
  attribute_added, attribute_removed}`.

#### Slice — customers

- `Customer` end-user body: id, external_id, email_verified,
  tenant_id, tenant_role (`admin|member|viewer`), plan_tier
  (`free|basic|pro|enterprise`), mfa_enabled, description,
  timestamps. `CustomerAttribute` 1→N with its own timestamps. No
  is_locked column — that's on Principal.
- Strict 3-field PATCH (email_verified, mfa_enabled, plan_tier).
  PATCH emits a single `updated` event listing the sorted set of
  changed field names. email_verified transitions trigger Principal
  status recompute.
- REST under `/api/v0/customers`: list, create, bulk upsert, get,
  patch, list/add/remove attributes.
- Events: `inventory.customer.{created, updated, bulk_upserted,
  attribute_added, attribute_removed}`.

#### Slice — employee_records (records, attribute mappings, matches, resolver)

- `EmployeeRecord` source-side row keyed on `(application_id,
  external_id)`, with cascading `EmployeeRecordAttribute`.
- `EmployeeProviderAttributeMapping` — per-application mapping from
  a record source key to a **canonical Person attribute key**
  (`person_key`), with `is_determinator` (drives resolver lookup) and
  `allow_upstream` (peer-record traversal edge). Unique on
  `(application_id, employee_record_key)`.
- `EmployeeRecordMatch` — 1:1 with EmployeeRecord, binding a record
  to a canonical **(Person, Employment)** pair plus a flag for
  whether the bridge was a direct determinator or upstream peer.
- REST under `/api/v0/employee-records`: list, create, bulk upsert
  (application referenced by `application_code`), get, list/add/
  remove attributes, manual match management
  (`GET/PUT/DELETE /employee-records/:id/match`), automated resolve
  (`POST /employee-records/:id/resolve`). Mapping CRUD lives under
  `/api/v0/applications/:appID/employee-record-mappings` and
  `/api/v0/employee-record-mappings/:id`.
- Resolver (`resolver.go`) two-pass matching:
  - Pass 1 — direct determinator (ANY-match). For each
    `is_determinator=true` mapping whose source key is present, look
    up a Person by `(person_key, value)`; if none exists, materialise
    a fresh Person seeded with the determinator attribute AND a fresh
    Employment (`code='active'`, `start_date=today`, no end_date) as
    the mask the record binds to.
  - For an existing Person, the resolver picks the first currently-
    active Employment as the binding mask
    (`PrimaryEmploymentForPerson`).
  - Pass 2 — upstream peer traversal: walk peer records sharing a
    `(key, value)` under an `allow_upstream=true` mapping, recurse
    with a visited-set to guard cycles.
  - Non-determinator mapped attributes propagate to the canonical
    Person on every successful match.
- The resolver itself never writes EmployeeRecordMatch — the service
  does, then emits
  `inventory.employee_record.{matched, unmatched}`.
- Events: `inventory.employee_record.{created, bulk_upserted,
  attribute_added, attribute_removed, matched, unmatched}`.

#### Slice — principals

- `Principal` polymorphic identity row over Employment / Workload /
  Customer with a `kind` discriminator, three partial FK columns
  (`principal_employment_id`, `principal_workload_id`,
  `principal_customer_id`), kind-specific `status` vocabulary,
  `is_locked` boolean, and `(kind, external_id)` uniqueness.
- Check constraints in the migration: exactly one `principal_*_id`
  set, `kind` ↔ matching FK, `status` ∈ kind vocabulary (employment
  status accepts any non-empty 64-char string; workload + customer
  bound to their universal vocabularies). Partial unique indexes on
  each `principal_*_id` enforce 1:1 body binding.
- `status_derivation.go` derives lifecycle status from current body
  state:
  - employment → `Employment.code` verbatim (or `terminated` when
    the row is gone)
  - workload   → `active` when the row exists, `expired` otherwise
  - customer   → `active` when email_verified, `registered` otherwise
  Lock is intentionally NOT part of derivation; it is a separate axis
  set explicitly via `POST /principals/:id/lock`.
- `RecomputeForBody(kind, bodyID)` is the cross-slice entry point:
  employments / customers call it on lifecycle / verification changes;
  it diffs current vs derived status and writes-back + emits
  `inventory.principal.status_recomputed` only on actual change.
- REST under `/api/v0/principals`: list, create (with derive-on-omit),
  get, `POST /:id/lock` (with optional reason), `POST /:id/unlock`.
  Lock/unlock are idempotent: re-locking an already-locked principal
  emits no event.
- Events: `inventory.principal.{created, status_recomputed, locked,
  unlocked}`.

#### Migrations

- Seven new bun migrations applied as a single group: `persons`,
  `org_units`, `employments` (`person_id`, `code`, `start_date`,
  `end_date`, partial index on active rows), `workloads`
  (`owner_employment_id`), `customers`, `employee_records` (+ record
  attribute / provider attribute mapping with `person_key` / 1:1
  match table with `person_id + employment_id`), `principals` (+
  `principal_attributes`, full check-constraint set, `is_locked`
  column, partial unique indexes on each body column).
- Eighth migration backfills `created_at` / `updated_at` columns on
  `persons` and `workloads` (DEFAULT NOW()) plus
  `(updated_at DESC)` indexes so paginated lists are stably ordered
  newest-first.

#### Additional endpoints

- `GET /api/v0/employee-record-matches` — returns every
  `EmployeeRecordMatch` row in one shot so clients can enrich a
  records list with its resolved (person, employment) without N+1
  per-record lookups.
- `GET /api/v0/principals/:id/attributes` — exposes the existing
  `principal_attributes` table (the row-level cross-body tagging) over
  REST, mirroring the persons / employments / workloads attribute
  surfaces.

### Changed

- `cmd/backplane/main.go` — composition root wires eight inventory
  slices end-to-end with cross-slice adapters that keep each slice
  decoupled:
  - `persons.Service`, `org_units.Service`, `employments.Service`,
    `workloads.Service`, `customers.Service`,
    `employee_records.Service` + `Resolver`, `principals.Service`.
  - Adapters bridge persons ↔ employments, org_units ↔ employments,
    applications ↔ workloads/employee_records, employments ↔
    workloads (owner-checks) and employee_records (employment
    membership), employments/customers ↔ principals (status
    recompute), and the employee_records resolver to a composed
    persons + employments API (the resolver materialises Person +
    Employment in one shot when no canonical row exists yet).
  - All inventory routes mounted under `/api/v0`.
- `internal/inventory/` replaces the previous empty
  `internal/domain/.gitkeep` placeholder.
- Persons / workloads / customers / principals List repositories now
  sort by `(updated_at DESC, id ASC)` instead of `external_id ASC`,
  so paginated UIs see the freshest rows first with stable
  tiebreaking. Other slices keep their previous orderings.
- Persons and Workloads gain `created_at` / `updated_at` columns and
  their services stamp `now()` on Create / Patch / BulkUpsert paths
  (workloads also propagate `updated_at` through the conflict upsert
  set).

## [0.2.0] — 2026-03-17

### Added

#### Integrations layer (`internal/integrations/`)

- New `integrations/` layer between `platform/` and `engines/` for the
  catalog of governed systems and the runtime registry of adapters that
  serve them.
- `integrations/applications/` — `Application` domain entity (bun model
  on Postgres) with CRUD service, partial-patch payloads, REST handlers
  under `/api/v0/applications`, decommission flow that emits
  `inventory.application.decommissioned`, and a matching helper that
  surfaces eligible connectors for an application's required tag set.
- `integrations/connectors/` — `ConnectorInstance` registry,
  `CapabilityDescriptor` (operations, status transitions, supported
  fact kinds, account-disable cascades), pure tag-set `Selector`,
  `RegistrationConsumer` (durable topic consumer for self-registration
  and heartbeat messages), and REST handlers under
  `/api/v0/connector-instances`. Online window: 2 minutes; stale
  cutoff: 24 hours.
- Connector-specific `RPCClient` over the generic AMQP request/reply
  primitive: encodes the connector command shape (correlation_id,
  reply_exchange / reply_routing_key, operation, payload, optional
  trace fields, `result_storage_requested`); decodes the response
  envelope; handles non-ok status as typed `*ErrRPCStatus`; pulls large
  results out of the data lake via a `LakeReader` adapter when the
  remote peer returns `result_storage_ref`.

#### Core additions (`internal/core/`)

- `core/correlation/` — `X-Correlation-ID` carrier on `context.Context`
  with `WithID` / `ID` / `Ensure` helpers, so service code can stamp
  events / log entries / RPC calls with one trace id.
- `core/webserver/` — `X-Correlation-ID` HTTP middleware: echoes the
  header when present, generates a fresh UUID v4 otherwise, attaches
  to the request context, propagates into slog access logs.
- `core/rabbitmq/rpc_client.go` — generic AMQP request/reply primitive.
  Opens its own dedicated channel on the shared `*amqp.Connection`,
  declares the responses exchange and a private exclusive auto-delete
  reply queue, correlates outgoing publishes with incoming replies by
  `correlation_id`, surfaces explicit timeouts (default 60 s), and
  exposes `ReplyTarget()` so protocol wrappers can encode the reply
  target into the command body when the wire shape requires it.

#### Migration tooling

- `internal/migrations/` — central bun migration registry
  (`migrations.Migrations`). Schemas land as raw SQL inside Go
  migration files so future model edits do not retroactively change
  historical migrations.
- Initial migrations: `applications` and `connector_instances` tables
  with their full column shape, indexes, and unique constraints.
- `cmd/migrate/` — stand-alone runner with `init` / `up` / `down` /
  `status` commands. Reuses the same secret store as backplane.
- Makefile targets: `migrate-init`, `migrate-up`, `migrate-down`,
  `migrate-status`.

### Changed

- `cmd/backplane/main.go` — composition root now wires the
  integrations layer end-to-end: applications + connectors
  repositories, the connector RPC client, the registration consumer
  goroutine, the `/api/v0` router group, and the cross-slice
  `applications.MatchingProvider` adapter.
- `internal/core/config/rabbitmq.go` — adds
  `connector_registration_exchange` (default `aurelion.connectors.registry`)
  and `connector_registration_queue` (default `aurelion.connectors.registration`).
- `internal/core/webserver/webserver.go` — installs the
  correlation-id middleware and threads `correlation_id` into every
  access-log line.

### Fixed

- `integrations/applications/Repository.List` and
  `integrations/connectors/Repository.{List,ListOnline}` return a
  non-nil empty slice on empty result sets so JSON encoders emit `[]`
  instead of `null` — clients that pin to array shape (typed
  `ApplicationFromApi[]` / `ConnectorInstanceFromApi[]`) no longer
  crash on first refresh of an empty registry.

## [0.1.0] — 2026-03-13

Initial bootstrap skeleton of `aurelion-backplane`: a single-process Go
backend covering API, persistence, messaging, audit log delivery, and
orchestrator scaffolding.

### Added

#### Project

- Go 1.26 module `github.com/aurelion-solutions/backplane`.
- `LICENSE` (BUSL-1.1) and `CLA.md` carried over from the Aurelion
  trunk; SPDX headers on every Go file.
- `.env.example`, `.secrets.example.json`, `.gitignore`, `Makefile`,
  top-level `README.md` with layout and dependency-direction rules.
- File-based `.secrets.json` stores native JSON objects per key —
  hand-editable, no escaped strings.

#### Core layer (`internal/core/`)

- `config/` — `Settings` aggregate with one file per section
  (`postgres.go`, `rabbitmq.go`, `app.go`), pure value types and a
  central `loader.go` with `decodeRequired` / `decodeOptional` helpers.
- `secret/` — `Manager` (read-only), `Mutator` (write), and
  `FullManager` (union) contracts; `ErrNotFound`, `ErrNotImplemented`.
- `logger/` — `slog.JSONHandler` factory with string-level parsing.
- `postgres/` — `bun.DB` factory with pgdriver pool, fail-fast `Ping`.
- `rabbitmq/` — connection + channel factory, typed `Exchange{Name, Type}`
  declarations (`Topic` / `Direct` / `Fanout` / `Headers` constants);
  generic `Consume` helper that declares queue, binds keys, and
  dispatches deliveries to a callback with ack/nack semantics.
- `webserver/` — `echo.Echo` factory with recover, request-id, CORS
  middleware, slog access log, and `/healthz`.
- `events/` — domain `Envelope` schema with `<domain>.<entity>.<operation>`
  routing-key grammar, `ParticipantKind`, `NewEnvelope` / `NewRoot` /
  `NewDownstream` constructors with validation; `Sink` interface, MQ
  publisher, and `Tee` fan-out helper.

#### Platform layer (`internal/platform/`)

- `secretmanagers/` — `Factory` registry, `File` provider with
  live-read + atomic temp/rename writes, `Stub` base, and stubs for
  `Vault`, `OpenBao`, `Akeyless`, `Conjur`.
- `siem/` — structured audit-log `Event` with frozen schema,
  trace/correlation propagation, `Sink` / `Reader` contracts, `Factory`
  registry, `Stub` base, real `File` (JSONL append + mutex) and `MQ`
  (topic publish with routing key `<component>.<level>`) sinks,
  `Stdout` (JSON-per-line), `MultiSink` fan-out, `EmitInfo` lifecycle
  helper, plus stubs for `ELK`, `Fluentd`, `Loki`, `Nagios`, `QRadar`,
  `Rsyslog`, `Seq`, `Splunk`, `Zabbix`.
- `storage/` — data-lake batch contract (`WriteBatch` / `ReadBatch` /
  `DeleteBatch`), `Factory`, `Stub` base, `File` provider writing
  per-dataset JSONL batches with path-traversal validation, and stubs
  for `S3` and `Iceberg`.
- `llm/` — chat-streaming `Provider` interface (channel-based
  `Stream`), `Message`, `Chunk`, `Role`, `Factory`, `Stub` base, and
  stubs for `LlamaCpp` (on-prem GGUF), `Anthropic`, `OpenAI`.

#### Engines (`internal/engines/`)

- `orchestrator/` — skeleton with domain `types` (`Pipeline`, `Step`,
  `Run`, `RunStep`, `RunStatus`, `StepStatus`), ports (`Repository`,
  `Loader`, `Dispatcher`, `StepExecutor`), `Service` API with
  `ErrNotImplemented` stubs (`StartRun`, `GetRun`, `CancelRun`,
  `ReportStepResult`), and `Runner` heartbeat loop.

#### Binaries (`cmd/`)

- `backplane/` — composition root. Wires secrets → config → Postgres
  → RabbitMQ → events → storage → SIEM (multi-sink: file + stdout) →
  LLM → webserver. Retries Postgres and RabbitMQ connection in a loop
  on failure (5 s interval, cancellable). Emits lifecycle Events
  through MQ on start/stop.
- `worker/` — stand-alone orchestrator runner. Skeleton heartbeat loop;
  emits lifecycle Events through MQ on start/stop.
- `log-siem-transmitter/` — bridges the `aurelion.logs` topic
  exchange (queue `aurelion.logs.siem`, `#` binding) to the configured
  `siem.Sink`. Multi-sink ready (default: `file`-only — `stdout` is
  excluded on purpose since this consumer's terminal is not the
  publisher's). Includes `README.md` and start-time banner.
- `log-dev-projector/` — in-memory log viewer for local development.
  Consumes `aurelion.logs.buffer` queue into a fixed-size ring with
  FIFO eviction and serves `GET /buffer?limit=N` + `GET /healthz` on
  `:8001`. Includes `README.md` and start-time banner.

### Architectural rules established

- **Module layering**. `cmd/*` is the only composition root; `core/*`
  holds single-implementation infrastructure; `platform/*` holds
  switchable backends; `engines/*` holds capability engines.
- **Dependency direction**. `cmd → engines → platform → core`. Core
  packages never import `core/config`; the composition root assembles
  every `Config` value and hands it in.
- **Env-vars budget**. The only env reads in the whole service are
  `AURELION_SECRET_PROVIDER` and `AURELION_SECRETS_FILE`. Everything
  else lives in code constants or in the secret store.
- **Secret-vs-app split**. Secret store holds credentials for external
  systems; application settings live inside the application.
- **Port/adapter split for switchable infra**. The contract lives in
  `core/secret`; implementations live alongside the factory inside
  `platform/secretmanagers/`.
