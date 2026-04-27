# employments

Period of work for a Person — one row per "mask / position". A single
Person can hold several Employments concurrently (full-time + part-
time) or sequentially (career history).

Lock / access posture intentionally **does not live here** — that is
the Principal layer's job (`principals`), a single kind-agnostic point
where access is granted or revoked. Employment carries the lifecycle
(`code`, `start_date`, `end_date`); Principal carries the posture.

Each Employment carries an EAV-style `employment_attributes` sidecar
for period-specific tags (job_title, manager_external_id, department
label, headcount allocation).

## Ingest contract

Employments **are not ingested directly**. They are created or
refreshed as a side effect of the `dataset_type = employee` contract
on [`persons`](../persons/), which carries the employments array
inline. Lineage from a raw lake record back to a specific Employment
is held by [`employment_record_matches`](../employment_record_matches/).
