# llm_classification mechanism

Uses an LLM (optionally with RAG retrieval) to classify free-form
facts that declarative rules can't reason about directly. The typed
output is fed back into the rest of the policy assessment pipeline as
enriched input or signals.

**Output class:** depends on prompt. Most commonly reactive **anomaly**
(`Decision.RiskLevel + Signals`) — the LLM produces a label + rationale
that lands as a structured signal `{"kind": "llm_classification", ...}`.
Can also emit a gate `Decision.Effect` when the prompt is explicitly
boolean.

## When to use

- "Is this Salesforce role description privileged?" → boolean.
- "Classify this group description as PCI / GDPR / SOX scope." → label.
- "Summarise why this finding is critical." → explanation text for
  UI.

## When NOT to use

- Anything that can be answered with structured rules (use `cedar` /
  `sod` / `risk_scoring`).
- High-volume scan over millions of records (LLM cost / latency
  prohibitive — use `behavioral` or `graph_analysis`).

## Inputs

- **Prompt template** sibling-to-manifest in the cartridge
  (`<rule>.prompt`), referenced via `Manifest.Body["prompt_template_file"]`.
- **`Facts.Principal` / `Facts.Target` / `Facts.Resource` metadata**
  to interpolate into the prompt.
- **Optional retrieval corpus** — resource catalog, compliance docs,
  historical findings.

## Algorithm

1. Render the prompt from template + `Facts`.
2. Run retrieval against the configured corpus (when configured).
3. Call the LLM via `core/llm`.
4. Parse the structured response against the manifest's
   `body.response_schema`.
5. Emit:
   - `Decision.Signals` = list with one structured dict
     `{"kind": "llm_classification", "label": "...", "confidence": <n>,
     "rationale": "..."}` — fits the polymorphic `Signals` shape.
   - `Decision.RiskLevel` mapped from the classification when
     applicable.
   - `Output.Evidence` = retrieval excerpts that informed the answer.
   - `Output.Confidence` = LLM-reported confidence.

Downstream rules (e.g. a Cedar rule reading
`context.llm.privilege_label == "privileged"`) consume this via the
`Facts.Threat` / `Facts.Extra` enrichment hook on the next step.

## Manifest shape

```jsonc
{
  "rule_id":   "glyph.llm.classify_role_privilege",
  "version":   1,
  "name":      "Classify role privilege",
  "mechanism": "llm_classification",
  "severity":  "info",
  "tags":      ["scan", "llm"],
  "body": {
    "prompt_template_file": "classify_role_privilege.prompt",
    "response_schema": {
      "type": "object",
      "properties": {
        "is_privileged": {"type": "boolean"},
        "confidence":    {"type": "number"},
        "rationale":     {"type": "string"}
      }
    },
    "retrieval": {"enabled": true, "corpus_id": "glyph.roles", "top_k": 5},
    "provider":  "anthropic",
    "model":     "claude-sonnet-4-6"
  }
}
```

## Supporting infrastructure

- `core/llm` — provider abstraction (Anthropic / OpenAI / LlamaCpp).
- Prompt template registry (filesystem-loaded; lives next to
  manifest).
- Embedding store / retriever — pgvector or DuckDB+Iceberg, future
  decision.
- Output schema validators (`santhosh-tekuri/jsonschema`).
- Confidence calibration.

## Status

Skeleton. Ships when `core/llm` infrastructure is finalised.
