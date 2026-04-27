# normalize.employee

Action for `dataset_type = employee`. Identity resolution: pull
records already deposited in the lake from various connectors and
assemble them into a canonical **Person** + one **Employment** per
incoming source record, via a determinator resolver.

> Not for access. This action knows nothing about roles, files or
> ACLs — those go through
> [`access_grant_record`](../access_grant_record/).

## Why

One human exists in several systems at the same time:

- BambooHR (HRIS) knows John as `bamboo_email = john@corp.com`
- MicrosoftAD knows him as `userPrincipalName = john@corp.com`

We need **one** canonical `Person` with every source attached via
`employment_record_matches`. Matching on email alone is brittle —
either "matched / not matched" on a single field, or parallel
duplicates when sources drift out of sync.

The fix is **determinator mappings per provider**, stored in Postgres
and edited by an admin.

## Model

Lake (raw):

```
EmployeeRecord     dataset_type=employee, payload JSONL
                   identified by external_id within source/provider
```

Postgres (reuses the existing `persons`, `person_attributes`,
`employments` tables):

| Table | Holds |
|---|---|
| `persons` | `(id, external_id, full_name)` — root identity |
| `person_attributes` | EAV: `(person_id, key, value)`, unique `(person_id, key)` |
| `employments` | `(id, person_id, code, start_date, end_date, ...)` — one "mask / position" of a Person (existing backplane model) |
| `employment_record_matches` | lineage: `(employment_id, source, source_record_external_id, matched_via_determinator)`, unique `(source, source_record_external_id)` |
| `employee_provider_mappings` | resolution rules: `(provider, record_key, person_key, is_determinator, allow_upstream)` |

**One record → one match row.** One Employment may carry several
match rows (HRIS + AD both describing the same job). Lock / access
posture lives in the Principal layer (`principals`), not here.

## Mapping rules

`employee_provider_mappings`, one row per
`(provider, record_key)`:

```
provider          = "bamboohr"
record_key        = "bamboo_email"
person_key        = "work_email"
is_determinator   = true
allow_upstream    = false
```

Two booleans set the **provider's role in the merge**:

| `is_determinator` | `allow_upstream` | Behaviour |
|---|---|---|
| `true`  | `false` | **Primary identity source** (usually HRIS). May create new Persons; creates new Employments under itself. |
| `false` | `true`  | **Secondary source** (AD, Okta, SAP). Never creates a Person — only attaches to an existing one via its key, and adds a match row against the existing Employment. |
| `false` | `false` | The attribute is just propagated into `person_attributes`; takes no part in matching. Pure metadata. |

`is_determinator=true` without `allow_upstream` is deliberately not
allowed on a secondary source: otherwise an AD dump with a brand-new
`userPrincipalName` would create a parallel Person that never
reconciles with a future HRIS record for the same human.

## Algorithm — per record

1. Read the record from the lake (`external_id`, `payload`).
2. **Idempotency check**: if `employment_record_matches` already has
   a row for `(source, external_id)`, skip (`AlreadyMatched++`).
3. Load mappings WHERE `provider = source AND is_active = TRUE`. If
   empty, `Skipped++`.
4. **Direct determinator** (provider is primary — has at least one
   `is_determinator=true` mapping):
   - for each determinator mapping: take the value from
     `payload[record_key]`, look up an existing Person by
     `(person_key, value)` in `person_attributes` via
     `persons.AttributeLookup`.
   - On hit → match, `matched_via_determinator=true`.
   - On miss, at the first determinator-keyed non-empty value:
     create
     `Person(external_id=value, full_name=payload[full_name|name|display_name])`
     + initial `PersonAttribute(person_key=value)`.
     `PersonsCreated++`.
5. **Upstream** (provider is secondary): for each `allow_upstream=true`
   mapping, look up a Person by `(person_key, value)`. On hit →
   match, `matched_via_determinator=false`. If every lookup misses,
   `Unresolved++`.
6. After the match: for every **non**-determinator mapping, UPSERT
   `person_attributes(person_id, key=person_key, value=...)`.
7. **Resolve Employment**:
   - primary: SELECT `employments WHERE person_id=? AND code=? AND start_date=?`. Miss → INSERT with defaults (`code` from payload or `"active"`; `start_date` from payload or today UTC).
   - secondary: SELECT any Employment for this Person (oldest first). None at all → `NoEmployment++`, skip (wait for the primary).
8. INSERT `employment_record_matches`.

All writes go through `ctx.Tx` — the runner rolls back the
transaction on a step error.

## Cross-application example

`bamboohr` arrives first:

- record1: `external_id=BMB-42, bamboo_email=john@corp.com, full_name="John Smith", department="Finance"`
- mapping `bamboo_email → work_email` (`is_determinator=true`)
- No Person found with `work_email=john@corp.com` →
  `Person(P-1, external_id=john@corp.com, full_name=John Smith)` +
  `PersonAttribute(P-1, work_email, john@corp.com)`
- Non-determinator attrs: `full_name=John Smith`, `department=Finance`
- `Employment(E-1, person=P-1, code=active, start_date=today)`
- `employment_record_matches(E-1, bamboohr, BMB-42, determinator=true)`

Then `microsoft-ad`:

- record2: `external_id=S-1-5-21-1001, userPrincipalName=john@corp.com, displayName="John H. Smith"`
- mapping `userPrincipalName → work_email`
  (`is_determinator=false, allow_upstream=true`)
- Person `P-1` found via `person_attributes(work_email=john@corp.com)`.
- Non-determinator attrs: `display_name=John H. Smith` (does NOT
  overwrite `full_name`!)
- Existing Employment `E-1` found for `P-1` (secondary does not
  create a new one).
- `employment_record_matches(E-1, microsoft-ad, S-1-5-21-1001, determinator=false)`

If bamboo writes `john@corp.com` while AD writes
`john.smith@corp.com`, upstream cannot help — you need additional
normalisation (lowercase + alias table) or a different `person_key`
(e.g., `employee_id` from HRIS, propagated into an AD
extensionAttribute).

## Events

| Type | When |
|---|---|
| `inventory.normalize.employee.completed` | finished a batch |

(Per-record events are reserved for the future, when finer hooks
become useful.)

## What it does NOT do

- normalize access — that is [`access_grant_record`](../access_grant_record/)
- attach accounts to Persons — a separate engine (account →
  principal matcher) runs AFTER normalize
- merge two existing Persons (conflict resolution, a separate story
  — usually an admin action)
- write to the lake — read-only
- modify `Employment` after creation (a fresh `code` / `start_date`
  in payload is currently ignored; an applier will land when needed)

## Source of truth

The algorithm is a 1:1 port from the Fortevera project
(`backend/src/app/org/services/employee_resolver.py`) and the
kernel's `EmployeeResolverService`. Adaptation:

- `Composite` → `Person` (plus an upstream-FK layer)
- `Candidate` → `EmployeeRecord` in the lake (not Postgres)
- `CompositeCandidate` (match) + Fortevera's separate `LaborPhase`
  events → one entity here: `employment_record_matches`, while the
  existing `Employment.code` carries the labor state as free text
- Person-side attributes — EAV in `person_attributes`, identical to
  Fortevera
