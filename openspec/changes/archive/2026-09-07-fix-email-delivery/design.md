## Context

`internal/email/pipeline.go` writes an `email_log` row with `idempotency_key` (UNIQUE), `status` (pending/sent/failed), and recipients; `retry_worker.go` ticks every 60s over pending rows. The retry path regressed to a no-op: it only increments `attempt_count`. Digests render from template vars that the caller never sets.

## Goals / Non-Goals

**Goals:**
- Pending emails are eventually sent or marked failed — never silently dropped.
- Digest email shows real content.
- SMTP operations cannot hang indefinitely.
- Unsubscribe present on digest/marketing mail.

**Non-Goals:**
- Not replacing the SMTP/Resend transport selection logic.
- Not redesigning template pipeline beyond variables needed for digest + unsubscribe.
- Not adding an email analytics dashboard.

## Decisions

### 1. Retry actually re-sends

`retry_worker` must reconstruct the send from the persisted row. Currently `email_log` stores recipient + idempotency key; ensure it also stores the rendered body (or a pointer to re-render inputs). Decision: persist the fully rendered body (HTML/text) on the row at enqueue time — simplest, deterministic, and survives template changes. On each retry: call `Sender.Send`; on success `status='sent'`; on failure increment `attempt_count`, apply backoff, and after `MAX_ATTEMPTS` (3) set `status='failed'` and emit a metric + (if configured) ops alert. Test must assert a real `Send` call happens.

### 2. Idempotency race fix

The enqueue flow: `INSERT ... ON CONFLICT(idempotency_key) DO NOTHING`; on conflict, read the existing row. Only treat as success if `status='sent'`. If `pending`, it is a retry candidate (continue, do not drop). If `failed`, either re-send or leave failed with a log. This removes the silent-loss path.

### 3. SMTP per-command deadlines

After `DialContext`, set `conn.SetDeadline(time.Now().Add(cmdTimeout))` before each MAIL/RCPT/DATA/QUIT, clearing after each response. `cmdTimeout` configurable (default 10s). This bounds every relay interaction.

### 4. Digest content from ContentJSON

`digest_worker` already has `ContentJSON` (aggregated digest output). Parse it into typed structs and pass top ideas, top questions, sentiment direction, sentiment change %, and links into the template data map so the existing template variables render. Add a rendering test asserting non-empty lists for a sample digest.

### 5. Unsubscribe support

Add `List-Unsubscribe` header (`<mailto:unsubscribe@…?subject=…>` and/or `https://…/unsubscribe?token=…`) on digest emails. Add a minimal `PATCH /api/user/preferences`-backed unsubscribe link (endpoint exists) — reusing the existing preferences opt-out, signing via `email/signer.go`. Marketing templates share a footer with the link.

### 6. Escape channel names

Use `html.EscapeString(channelName)` (or template-driven rendering) in the Resend legacy path and any other place channel/user content is interpolated into HTML.

## Risks / Trade-offs

| Risk | Mitigation |
|---|---|
| Re-rendering sends different body than enqueued | Persist body at enqueue; retry sends exactly what was enqueued |
| SMTP deadlines kill slow-but-legit relays | Configurable timeout (10s default); only affects stalled commands |
| ContentJSON schema drift breaks template fill | Defensive parsing: on parse failure send digest with graceful "no items" + log, never crash |
| Unsubscribe link needs auth | Reuse signed-URL secret (HMAC) so link is self-authenticating, matches existing pattern |
