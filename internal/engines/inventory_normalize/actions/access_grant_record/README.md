# normalize.access_grant_record

Action for `dataset_type = access_grant_record`. Stateless
projection: take raw assignment facts from the lake and **translate
them from the application's vocabulary into the business vocabulary**
through a dictionary of mapping rules.

Pure function — no Postgres lookups, no hidden state:

```
project(AccessGrantRecord, [active CapabilityMapping]) -> [CapabilityGrant]
```

A record may produce 0 (no mapping covers it), 1, or N CapabilityGrants
(one record may match several mappings).

## Data layers

```
Lake (raw from the connector):
  - AccessArtifact     — resource description (Finance-Admins, F_BKPF_BUK, ...)
  - AccessGrantRecord  — assignment fact (john ∈ Finance-Admins)

Postgres (normalized):
  - CapabilityGrant    — projection of AccessGrantRecord into the
                          business vocabulary via CapabilityMapping rules
```

`AccessArtifact` and `AccessGrantRecord` are **two different**
entities in the lake:

- artifact describes what exists in the application (with attributes
  like `classification`, `company_code`, `env`)
- grant_record describes who is attached to it (`account`, `action`)
- they are stored separately because resource attributes change
  independently from assignments; one resource may be assigned to
  many accounts

The projector uses both: `AccessGrantRecord` provides
`(account, resource, action)`; resource attributes are read directly
from the lake (DuckDB scan over `lake/access_artifact/`) when a
mapping requires `scope_value_source = resource_attribute`. There is
no dedicated `access_artifact` action — billions of resource
descriptions are not worth duplicating into Postgres; they stay
lake-only.

Symmetry with the people side:

| | Lake (raw) | Postgres (normalized) |
|---|---|---|
| People | `EmployeeRecord` | `Person` + `Employment` |
| Access — resources | `AccessArtifact` | — (lake only) |
| Access — assignments | `AccessGrantRecord` | `CapabilityGrant` |

## Why

Every application speaks its own language:

- AD: "membership in the `Finance-Admins` group"
- NTFS: "Read+Write on `\\fs\HR\salary.xlsx`"
- PostgreSQL: "GRANT SELECT on `production.users`"
- SAP: "authobj F_BKPF_BUK with company_code=1000"

The business speaks its own:

- "may manage finance data"
- "may edit HR documents"
- "may read prod data"
- "may post financial documents for company 1000"

A mapping rule is a dictionary translating the first vocabulary into
the second. Admins write them by hand, one per resource pattern.

## NB: principal

In the lake, `AccessGrantRecord.account` is **an application user-
mailbox** (`account.username` or `account.id`), **not** a principal
(Person / NHI). The `account → principal` linkage is established
**LATER**, by a separate engine that runs after projection.

Our `CapabilityGrant` therefore carries `account_id`, not
`principal_id`. The `account ↔ principal` resolver fills the
principal in afterwards.

In the examples below, `john`, `jane`, `alice`, `bob` are
`account.username` values, not people. What real human stands behind
the account is a separate question, irrelevant at projection time.

## Mapping rule

Stored in `normalize_capability_mappings`:

```
capability_id        = "manage_finance_data"   -- business id
application_id       = <ad-app-uuid>           -- application filter
action_slug          = "member_of"             -- action filter
# XOR — exactly one of three:
resource_id          = NULL
resource_kind        = "ad_group"
resource_path_glob   = "Finance-*"
scope_key            = "department"
scope_value_source   = {"kind": "constant", "value": "finance"}
is_active            = true
```

`scope_value_source` — where to take the scope value for the output
CapabilityGrant. Five kinds:

- `constant` — hard-coded value
- `application_id` — take the application id itself
- `principal_attribute` — take from the resolved account's attrs
  sidecar (`cost_center`, `department_code`); by the time projection
  runs the account has been bound to a Principal, so these attrs
  describe that principal
- `resource_external_id` — take the resource external_id straight off
  the grant record (the common case for "scope IS the thing":
  AD group SID, ACL share id, SAP role code, …)
- `resource_attribute` — take from the resource's attributes
  (`classification`, `company_code`); not implemented yet (needs lake
  lookup into AccessArtifact)

## Algorithm for one AccessGrantRecord

1. Find every active CapabilityMapping.
2. For each, check:
   - `application_id` filter (if set)
   - `action_slug` filter (if set)
   - resource match (XOR: `resource_id` / `resource_kind` /
     `resource_path_glob`, via `fnmatch`)
   - resolve `scope_value` from `scope_value_source`
3. For every matching mapping, emit a `CapabilityGrant`:
   - natural key:
     `(account_id, capability_id, scope_key_id, scope_value)`
   - lineage: `source_access_grant_record_id`,
     `source_capability_mapping_id`
4. Upsert into `capability_grants`.

Idempotent: replaying the same grant with the same mapping set
changes nothing.

## Four examples

### 1. Role (AD Group)

Lake:
```
AccessArtifact:     { kind: ad_group, name: "Finance-Admins" }
AccessGrantRecord:  { account: john, resource: "Finance-Admins", action: "member_of" }
```

Mapping:
```
resource_kind        = "ad_group"
resource_path_glob   = "Finance-*"
capability_id        = "manage_finance_data"
scope_key            = "department"
scope_value_source   = { kind: constant, value: "finance" }
```

Result:
```
CapabilityGrant(account=john, capability=manage_finance_data, department=finance)
```

### 2. File (NTFS on a file share)

Lake:
```
AccessArtifact:     { kind: file, path: "\\fs\HR\salary.xlsx", classification: "pii" }
AccessGrantRecord:  { account: jane, resource: "\\fs\HR\salary.xlsx", action: "write" }
```

Mapping:
```
resource_kind        = "file"
resource_path_glob   = "\\fs\HR\*"
action_slug          = "write"
capability_id        = "modify_hr_documents"
scope_key            = "data_class"
scope_value_source   = { kind: resource_attribute, key: "classification" }
```

Result:
```
CapabilityGrant(account=jane, capability=modify_hr_documents, data_class=pii)
```

A read-only grant could be covered by a separate mapping on
`action_slug=read` → `view_hr_documents`. Every action can get its
own capability if you want.

### 3. ACL (PostgreSQL)

Lake:
```
AccessArtifact:     { kind: db_acl, schema: "production", table: "users", env: "prod" }
AccessGrantRecord:  { account: alice, resource: "production.users", action: "select" }
```

Mapping:
```
resource_kind        = "db_acl"
resource_path_glob   = "production.*"
action_slug          = "select"
capability_id        = "read_prod_data"
scope_key            = "environment"
scope_value_source   = { kind: resource_attribute, key: "env" }
```

Result:
```
CapabilityGrant(account=alice, capability=read_prod_data, environment=prod)
```

### 4. SAP (where the spaghetti lives)

One SAP user-assignment is a **tree**:

```
bob assigned composite role Z_FI_APPROVER_GLOBAL
  ├─ single role SAP_FI_AP_PAYMENT
  │    ├─ authobj F_BKPF_BUK (company_code=1000)
  │    └─ authobj F_BKPF_BUK (company_code=2000)
  └─ single role SAP_FI_AR_REPORT
       └─ authobj F_KKBE_AB (company_code=1000)
```

**The connector** flattens the tree into a list of AccessGrantRecords
— one per authobj instance. Backplane does not know the tree and does
not need to; only the SAP connector does, via SU24 / USOBT_C.

Lake:
```
bob → permit sap_authobj F_BKPF_BUK (company_code=1000)
bob → permit sap_authobj F_BKPF_BUK (company_code=2000)
bob → permit sap_authobj F_KKBE_AB (company_code=1000)
... plus assume on the single roles, if interesting to the business
```

Mapping (at the authobj level):
```
resource_kind        = "sap_authobj"
resource_path_glob   = "F_BKPF_*"
action_slug          = "permit"
capability_id        = "post_financial_documents"
scope_key            = "company_code"
scope_value_source   = { kind: resource_attribute, key: "company_code" }
```

Result — **two** CapabilityGrants from a single role assignment:
```
CapabilityGrant(account=bob, capability=post_financial_documents, company_code=1000)
CapabilityGrant(account=bob, capability=post_financial_documents, company_code=2000)
```

Notes on SAP:

- composite roles usually **are not mapped** — they are containers.
  Map single roles (if narrow enough) or authobjs (when fine-grained
  accuracy matters).
- If an authobj has no mapping, it simply does not surface as a
  capability. The raw grant still sits in the lake, nothing is lost.
  An admin can write a rule later and the projector re-projects.

## Events

| Type | When |
|---|---|
| `inventory.normalize.access_grant_record.completed` | projector ran over a batch |
| `inventory.normalize.access_grant_record.grant_changed` | a specific CapabilityGrant was created, updated, or its scope_value changed |

## What it does NOT do

- attach accounts to principals — that is a separate engine (see
  "NB: principal" above)
- expand SAP composite roles — the connector does that via SU24
- aggregate grants per principal ("Bob has N capabilities in total")
  — that is analytics, not normalize
- answer "may X do Y" — that is policy engine territory, a
  completely different layer
- work with identity (`Person`, `Employment`) — that is
  [`employee`](../employee/)

## Source of truth

The algorithm is ported from the kernel's `CapabilityProjector`
([src/inventory/access_model/capability_grants/capability_projector.py](../../../../../../aurelion-kernel/src/inventory/access_model/capability_grants/capability_projector.py)).

Differences in backplane:

- **Input entity renamed**: the kernel's `AccessFact` (plus an
  intermediate `EffectiveGrant`) becomes a single `AccessGrantRecord`
  in the lake. The intermediate layer is collapsed because there is
  no Initiative concept on this side.
- **Subject replaced by account**: in the kernel,
  `EffectiveGrant.subject_id` pointed to an already-resolved
  principal. Here, `AccessGrantRecord.account` is a raw user-mailbox;
  the principal is attached by a separate engine after projection.
  `CapabilityGrant` therefore carries `account_id`, not
  `principal_id`.
