# access_profile

Read-only projection over the inventory layer. Given a Person, it
assembles the full human-access picture in one nested document. Owns no
table and emits no events.

It walks the existing inventory spine —

```
person → employments → employment-principals → accounts → grants
                                             ↘ initiatives (justifications, with validity windows)
```

— joining the catalog (capabilities, scope keys, applications) for
human-facing labels. The account → principal edge it relies on is
[`accounts.principal_id`](../accounts/) (the assignment edge).

`Terminated` is computed at read time: a person is terminated when they
hold no active employment (every `end_date` is in the past) and have at
least one employment.

## Read surface

```
GET /api/v0/persons/{id}/access-profile
```

Read-only; safe on prefetch / HEAD — never writes. Assembles the
per-person human-access tree, grouped by application.
