# inventory_normalize.actions

One sub-package per `dataset_type` understood by the normalize
engine. Each is an orchestrator-registerable action that reads the
lake batch produced by `inventory_ingest` and projects it into the
relevant inventory tables.

| Action | Pair | Target table(s) |
|---|---|---|
| `account` | `("inventory_normalize", "account")` | `accounts` |
| `access_grant_record` | `("inventory_normalize", "access_grant_record")` | `capability_grants` |
| `employee` | `("inventory_normalize", "employee")` | `persons`, `employments`, `employee_record_matches`, `employment_record_matches` |
| `orgunit` | `("inventory_normalize", "orgunit")` | `org_units` |
| `person` | `("inventory_normalize", "person")` | `persons` |

Each sub-package owns its `README.md` describing the algorithm and
lake-record shape.
