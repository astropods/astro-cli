# Eval Dataset v2 — Judgment Criteria and Reasons

Extends `docs/01-spec/eval-dataset-v2-spec.md`.

## Summary

The v2 eval dataset flow already lets reviewers label traces as `good`, `bad`, or `unknown`. `good` and `bad` become Langfuse dataset items; `unknown` removes the trace from the review queue without adding it to the dataset.

This spec adds judgment reasons to that flow. Reviewers explain why a `good` or `bad` trace belongs in the dataset by selecting fixed Astro-owned judgment criteria. There are no custom criteria in this version.

Criteria labels and display order are defined in the frontend. Valid criterion dimensions are defined as a server enum and validated before writes. Astro DB stores selected dimension keys and values, but does not store a criteria catalog table.

---

## Terminology

**Dataset judgment** is the reviewer label for eval usefulness:

- `good` — useful successful example, added to the dataset.
- `bad` — useful failure example, added to the dataset.
- `unknown` — reviewed but skipped, not added to the dataset.

**Judgment criterion** is a predefined quality reason reviewers can select when marking a trace `good` or `bad`.

**Criterion dimension** is the paired storage model for criteria. Each dimension has one positive `good` label, one negative `bad` label, and a stable `dimension_key`.

**Judgment reason** is the historical record that a dimension was selected for a specific trace judgment, with the value captured at judgment time.

---

## Goals

- Capture why a reviewer marked a dataset item `good` or `bad`.
- Define the fixed criteria and labels in astro-client.
- Validate submitted criteria in astro-server with a server-owned enum.
- Store selected criteria locally with the judgment.
- Store the selected dimension value on each judgment reason.
- Include compact judgment reason metadata on Langfuse dataset items.

## Non-goals

- No custom criteria.
- No criteria catalog table.
- No criterion editing, soft delete, or version history.
- No reason requirement for `unknown`.
- No multi-reviewer consensus or review history table.

---

## Criteria definitions

The criteria definitions are code-owned in this version.

Frontend owns labels and display order:

| Dimension key | Good label | Good value | Bad label | Bad value |
|---|---|---:|---|---:|
| `accuracy` | Correct info | `1` | Hallucination | `-1` |
| `completeness` | Complete | `1` | Incomplete | `-1` |
| `instruction_following` | Followed instruction | `1` | Ignored instruction | `-1` |
| `scope_clarity` | Clear & well-scoped | `1` | Unclear or poorly scoped | `-1` |
| `tone` | Appropriate tone | `1` | Inappropriate tone | `-1` |

Server owns the allowed dimension enum and rejects unknown dimension keys before writing reasons. Frontend and server enum values must match exactly; submitted labels are ignored.

`dimension_key` is the stable export key. A selected reason is identified by `dimension_key` plus `dimension_value`; for example, `accuracy = -1` means Hallucination and `accuracy = 1` means Correct info.

---

## Data model

### `eval_dataset_judgment_reasons`

New table. Stores one or more selected criteria for each `good` or `bad` judgment.

| Column | Type | Notes |
|---|---|---|
| `eval_dataset_id` | uuid | FK to `eval_dataset_judgments`. |
| `trace_id` | text | FK to `eval_dataset_judgments`. |
| `dimension_key` | text | Server-validated criterion dimension key. |
| `dimension_value` | numeric | Value captured at judgment time. Current human selections write `-1` or `1`; future producers may write any value in range. |
| `created_at` | timestamptz | Default `now()`. |

Constraints:

| Constraint | Purpose |
|---|---|
| `PRIMARY KEY (eval_dataset_id, trace_id, dimension_key)` | Prevents duplicate selected criteria for one judgment. |
| `FOREIGN KEY (eval_dataset_id, trace_id) REFERENCES eval_dataset_judgments(eval_dataset_id, trace_id) ON DELETE CASCADE` | Removing a judgment removes its reasons. |
| `CHECK (dimension_value BETWEEN -1 AND 1)` | Keeps stored values on the dimension score scale. |

There is no FK or CHECK constraint that restricts `dimension_key` to the criteria enum. The server validates `dimension_key` before insert/update.

For human review, `dimension_value` is derived from the submitted verdict: `1` for `good`, `-1` for `bad`.

The value is stored as a range from `-1` to `1` to support future LLM-as-judge output. Human-selected criteria are absolute signals, so they use only `-1` or `1`. An LLM judge may later write partial scores such as `0.4` or `-0.7` when it has weaker evidence for a dimension.

---

## Langfuse dataset item metadata

Existing Langfuse dataset item metadata is unchanged:

| Metadata field | Current value |
|---|---|
| `verdict` | `1` for `good`, `-1` for `bad` |
| `confidence` | `100` for human judgments |
| `judged_by_user_id` | Astro user id |
| `judged_at` | RFC3339 timestamp |

This spec adds one metadata field for `good` and `bad` judgments:

| Metadata field | Value |
|---|---|
| `judgment_criteria` | Array of selected `{ dimension_key, value }` objects. Empty array when the verdict has no reasons yet. |

Example:

```json
{
  "verdict": -1,
  "confidence": 100,
  "judged_by_user_id": "user_123",
  "judged_at": "2026-06-30T14:10:00Z",
  "judgment_criteria": [
    { "dimension_key": "accuracy", "value": -1 },
    { "dimension_key": "completeness", "value": -1 }
  ]
}
```

Langfuse metadata should describe the dataset item. Astro DB owns the judgment row and selected reason rows. `unknown` judgments write no Langfuse dataset item.

---

## Server API

| Method | Path | Change |
|---|---|---|
| `GET` | `/api/v1/deployments/:id/dataset` | Modified. Add derived criteria counts. |
| `POST` | `/api/v1/deployments/:id/dataset/judgments` | Modified. Accept selected criteria when submitting a judgment. |
| `PATCH` | `/api/v1/deployments/:id/dataset/judgments/:trace_id` | Modified. Change verdict only; clears the judgment's criteria. |
| `PUT` | `/api/v1/deployments/:id/dataset/judgments/:trace_id/criteria` | New. Replace the criteria for an existing judgment. |

### Dataset summary

Extend response:

| Field | Notes |
|---|---|
| `criteria_counts` | Array of `{ dimension_key, value, count }` rows derived from judgment reasons. `value` is the stored `dimension_value`. |

Server behavior:

Counts are derived from `eval_dataset_judgment_reasons`, not stored on `eval_datasets`:

```sql
SELECT dimension_key, dimension_value, COUNT(*)
FROM eval_dataset_judgment_reasons
WHERE eval_dataset_id = $1
GROUP BY dimension_key, dimension_value;
```

The response should include two count rows for every server-known dimension: one for `value = 1` and one for `value = -1`. If no judgments selected that side of the dimension, return `count = 0`.

### Submit judgment

Extend request:

| Field | Notes |
|---|---|
| `verdict` | Existing `good` \| `bad` \| `unknown`. |
| `criteria` | Optional array of selected dimension keys for `good` / `bad`; rejected for `unknown` unless empty. |

Server behavior:

1. Validate verdict.
2. For `good` / `bad`, validate any submitted criteria against the server enum.
3. For `unknown`, require no selected criteria.
4. Insert `eval_dataset_judgments`.
5. Insert selected `eval_dataset_judgment_reasons` when criteria are submitted, writing `dimension_value = 1` for `good` and `dimension_value = -1` for `bad`.
6. For `good` / `bad`, upsert the Langfuse dataset item with judgment metadata.
7. For `unknown`, write no Langfuse dataset item.

The local judgment row and reason rows should be written in one DB transaction before the Langfuse mutation so they act as the duplicate gate. If the Langfuse dataset item write fails, compensate by deleting the judgment row; the reason rows cascade through the FK.

### Change verdict

PATCH already accepts `{ verdict }` only and does not touch reasons. Reasons cascade only on a judgment row delete, so a verdict flip leaves stale reason rows behind today.

Request: unchanged (`{ verdict }`, required).

Changes:

- Clear the judgment's `eval_dataset_judgment_reasons` on any verdict change. Criteria are verdict-scoped, so the flip invalidates them; the frontend re-submits via the criteria endpoint. Needs a new store op that deletes reasons and returns the previous set for restore.
- Include an empty `judgment_criteria` array in Langfuse metadata on upsert.

PATCH should preserve the previous reasons before clearing them and restore them if a later Langfuse or count update fails and the judgment is rolled back.

### Replace criteria

PUT replaces the full set of reasons for an existing judgment. The judgment must already exist and its verdict must be `good` or `bad`.

Request:

| Field | Notes |
|---|---|
| `criteria` | Required array of selected dimension keys. Empty array clears reasons. |

Behavior:

1. Load the judgment. Reject if it does not exist or its verdict is `unknown`.
2. Validate submitted criteria against the server enum.
3. Replace `eval_dataset_judgment_reasons` with the submitted set, writing `dimension_value = 1` when the verdict is `good` and `dimension_value = -1` when the verdict is `bad`.
4. Upsert the Langfuse dataset item metadata with the new `judgment_criteria`.

PUT should preserve the previous reasons before replacing them. If the Langfuse mutation or count update fails, restore the previous reasons.

### Delete judgment

No request or response change. Deleting the judgment cascades reasons through the FK and removes the Langfuse dataset item for `good` / `bad`.

The handler should preserve the previous reasons before deleting the judgment. If a later Langfuse mutation or count update fails and the handler restores the judgment row, it must restore the previous reasons too.

---

## Key decisions

**Criteria are code-owned.** Astro-client owns labels and display order. Astro-server owns the validation enum. Astro DB stores selected reason rows, not a criteria catalog.

**Server validation replaces DB catalog constraints.** Astro-server validates submitted criteria before writing judgment reasons. The database does not constrain `dimension_key` to the enum.

**Frontend and server enums must match.** Astro-client submits dimension keys, not labels. Astro-server treats the keys as the contract and rejects unknown values.

**Reasons store dimension values.** Human judgments store `dimension_key + dimension_value`. This avoids separate good and bad keys for the same quality axis while preserving which side was selected.

**Dimension values use a range.** Current human-selected reasons write `-1` or `1`, but the column supports any value in `[-1, 1]` for future LLM-as-judge predictions where a dimension score may be directional but not absolute.

**Reasons are only for dataset items.** `unknown` removes a trace from the review queue, but it is not a good/bad dataset item and does not require reasons.
