# grammar

Single source of truth for the pipeline YAML JSON Schema.

## Contract

The embedded `schema.json` describes the structural shape of a
pipeline definition. Two consumers read from here and nowhere else:

- `core/orchestrator/loader` — validates each cartridge YAML during
  catalog rebuild.
- The well-known descriptor endpoint — serves the schema to clients
  (Studio, doc generators) that want to lint locally.

There is no parallel schema definition anywhere in the tree. Adding
one is an immediate review-fail.

## API

```go
schema := grammar.JSONSchema()  // []byte — the raw schema
loader.Validate(yamlBytes)     // uses the same schema internally
```

## What this package does NOT do

- Semantic validation (template references, step DAG cycles, etc.).
  That happens after structural validation, in `loader`.
- Versioning. There is one schema; pipelines that need newer fields
  bump it in place and migrate stragglers.
