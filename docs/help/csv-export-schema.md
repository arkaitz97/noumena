# CSV export schema

> Source of truth: `internal/export/encoder.go` (`CSVHeader`).
> Capability: [`specs/03-comment-ingestion.md`](../../specs/03-comment-ingestion.md).
> Related API: [`../api/comments.md`](../api/comments.md).

When you export comments from Noumena (`POST /api/comments/export`
with `format=csv`, or any of the insights export endpoints) you get
a UTF-8 CSV file with a fixed header and column order. **The order
will not change without a major-version bump** — BI tools and
spreadsheets that depend on column position can rely on it.

## Columns

| # | Column | Type | Nullable | Meaning |
|---|---|---|---|---|
| 1 | `id` | string | no | Stable internal comment ID (ULID-shaped). Use this to deduplicate across exports. |
| 2 | `channel_name` | string | no | Display name of the YouTube channel at export time. May change if the creator renames the channel. |
| 3 | `author_handle` | string | yes | The commenter's `@handle` if YouTube exposed it; empty when the API returned only a display name. |
| 4 | `body` | string | no | The full comment text, verbatim. May contain newlines, commas, and quote characters — RFC-4180-quoted. |
| 5 | `posted_at` | string (RFC 3339, UTC) | no | When the comment was posted on YouTube. Always UTC, e.g. `2026-04-19T18:31:42Z`. |
| 6 | `category` | string (enum) | yes | LLM-assigned category. One of `praise`, `question`, `complaint`, `suggestion`, `other`. Empty when the comment has not yet been processed. |
| 7 | `sentiment_score` | float (-1.0 … 1.0) | yes | LLM-assigned sentiment. `-1` is most negative, `0` is neutral, `1` is most positive. Empty when unprocessed. |
| 8 | `topic` | string | yes | LLM-assigned short topic label (free text, typically 1–4 words). Empty when unprocessed. |
| 9 | `confidence` | float (0.0 … 1.0) | yes | LLM's self-reported confidence in the category/topic assignment. Empty when unprocessed. |
| 10 | `processed_at` | string (RFC 3339, UTC) | yes | When the LLM processing pipeline finished labelling this comment. Empty when unprocessed. |

"Empty" means a literal empty CSV field, not the string `null`.

## Encoding details

- File encoding: **UTF-8**. No BOM. Excel users on Windows may need
  to import via the Data → From Text wizard and explicitly select
  UTF-8 to avoid mojibake on emoji.
- Line endings: `\r\n` between rows (RFC 4180).
- Quoting: fields containing `,`, `"`, or `\n` are wrapped in
  double quotes; embedded `"` is escaped as `""`.
- Header row: always present, always first, always exactly as
  listed above.

## Stability guarantees

- Column order: **stable**. Will not change without a documented
  schema-version bump.
- New columns: may be added **at the end**. Consumers that pin to
  positional indexes 1–10 will keep working.
- Column rename or drop: a major-version bump only, with notice in
  the release notes and a deprecation window.

## Sync vs async exports

The CSV produced by a synchronous export (≤ 50,000 rows) and an
asynchronous export job (> 50,000 rows) is byte-identical for the
same query. The size threshold and the polling flow are documented
in [`../api/comments.md`](../api/comments.md).

## Common questions

- **"Why is `processed_at` empty for some rows?"** Those comments
  have been ingested but not yet labelled by the LLM pipeline —
  either they are still in the queue, or they were deferred after
  retries and will be picked up later. See
  [`specs/04-llm-processing.md`](../../specs/04-llm-processing.md).
- **"Can I get the YouTube comment URL?"** Not in the CSV today.
  You can reconstruct it from the channel and the YouTube comment
  ID — but the latter isn't currently exported. File a request if
  you need it.
- **"What about JSON exports?"** The async export endpoint returns
  CSV by default. JSON export is on the roadmap; until then, use
  the read-only insights API
  ([`../api/insights.md`](../api/insights.md)) for programmatic
  access.

## Related

- [`../api/comments.md`](../api/comments.md) — export API reference.
- [`../api/insights.md`](../api/insights.md) — read-only JSON access
  to the same data.
