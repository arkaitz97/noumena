## 1. Data and client foundation

- [x] 1.1 Add migration fields for uploads playlist ID, per-video sync watermark, and page checkpoint.
- [x] 1.2 Add typed `playlistItems.list` response and iterator with `fields` projection.
- [x] 1.3 Resolve/cache uploads playlist ID through `channels.list(part=contentDetails)`.
- [x] 1.4 Keep bounded `search.list` fallback and emit fallback telemetry.
- [x] 1.5 Preserve `totalReplyCount` and embedded reply count in thread results.

## 2. Quota-efficient ingestion

- [x] 2.1 Replace `ListVideos` discovery with uploads-playlist iteration.
- [x] 2.2 Gate `ListReplies` on missing replies only; prevent calls for zero-reply threads.
- [x] 2.3 Add per-video watermark and page checkpoint persistence.
- [x] 2.4 Stop comment pagination at the incremental watermark where ordering permits.
- [x] 2.5 Resume safely after deadline, process failure, or worker restart.

## 3. Quota control and scheduling

- [x] 3.1 Centralize YouTube method quota costs and request accounting.
- [x] 3.2 Add daily project budget and per-job request/page/video/reply ceilings.
- [x] 3.3 Stop and defer jobs on quota exhaustion without consuming retry budget.
- [x] 3.4 Add adaptive polling for active, inactive, and repeatedly empty channels.
- [x] 3.5 Preserve existing backoff, token fallback, and idempotent enqueue behavior.

## 4. Observability and tests

- [x] 4.1 Add request metrics by method, units, channel, status, and suppression reason.
- [x] 4.2 Add structured logs for quota decisions, fallback discovery, and checkpoint resume.
- [x] 4.3 Test playlist pagination, fields, caching, and fallback.
- [x] 4.4 Test reply gating and deep-reply pagination.
- [x] 4.5 Test quota budgets, retry classification, adaptive polling, and checkpoint recovery.
- [x] 4.6 Test unchanged comment persistence, parent IDs, depths, and deduplication.

## 5. Documentation

- [x] 5.1 Document YouTube quota costs, budget policy, and fallback behavior.
- [x] 5.2 Document stored checkpoint fields and operational reset procedure.
