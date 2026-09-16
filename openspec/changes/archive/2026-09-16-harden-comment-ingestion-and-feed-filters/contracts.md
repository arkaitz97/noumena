# Runtime Contracts

## YouTube Retrieval Scope

- API-key retrieval promises published, video-associated comments only.
- Creator OAuth retrieval may include creator-visible moderation states when configured for that channel.
- Channel-level rows without a video association are excluded from the video corpus and must be reported as skipped scope rows.
- Telemetry records effective credential scope, phase, resumed page count, incomplete ceiling stops, reply reconciliation, and skipped scope rows.

## FeedScope

`FeedScope` contains `channels`, `categories`, `categoriesSpecified`, `intent`, `insightType`, `dateFrom`, `dateTo`, `search`, `includeToxic`, `range`, and `view`.

- Omitted categories use `question`, `content_idea`, `collaboration_ask`, `praise`, `constructive_criticism`.
- Explicit `categories=` means match no categories.
- `ideas` means `content_idea OR video_idea`; `actionable` means at least one actionable insight.
- Date-only `date_to` is exclusive next UTC day; timestamp values remain exact.
- Search is literal substring matching; `%`, `_`, and `\\` are escaped before SQLite `LIKE`.
- Saved defaults omit ordinary category and toxicity filters.
