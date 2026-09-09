## Why

Email delivery has a critical silent-loss bug plus robustness gaps:

1. **Retry never sends** — `internal/email/retry_worker.go:116-173` never calls `Sender.Send`; the "retry" only increments `attempt_count` then marks failed at 3. Any email that hit a transient SMTP failure (`pipeline.go:176-186` leaves the row `pending`) is **never delivered** and silently dropped after 3 attempts. Weekly digests, verification, and password-reset emails are lost.
2. **Idempotency race loses emails** — `pipeline.go:163-171` treats a UNIQUE-violation on `idempotency_key` as "idempotent success" even when the first sender failed (row stays `pending`, never re-sent) → double-failure path loses mail with no error surfaced.
3. **Digest email renders empty** — `digest_worker.go:346-356` passes `ContentJSON` but `templates.go:285-303` renders `TopIdeas`/`TopQuestions`/`SentimentDirection`/`SentimentChangePct` which are never populated → recipients get "Sentiment trend: <no value>" and empty lists.
4. **SMTP can hang forever** — `smtp.go:47-117` only `DialContext` has a timeout; MAIL/RCPT/DATA/write have no per-command deadlines → a stuck relay hangs a goroutine/request indefinitely.
5. **No unsubscribe** — 13 templates, none include `List-Unsubscribe`/link; weekly digest is quasi-marketing.
6. **Unescaped channel name** — legacy Resend path `client.go:145-165` injects YouTube-supplied `channelName` into HTML via `fmt.Sprintf`, unescaped → HTML injection in mail.

## What Changes

- Retry worker actually re-sends persisted recipient/rendered body; mark `sent` only on success; `failed` after capped attempts.
- Fix idempotency race: on UNIQUE violation, read the row's status; dedupe only when `sent`, otherwise retry/re-send.
- Add per-command deadlines on the SMTP connection.
- Populate digest-email template variables from `ContentJSON` (parse and pass top ideas/questions/sentiment).
- Add `List-Unsubscribe` header + link to digest/marketing templates.
- `html.EscapeString` (or a proper HTML builder) for channel names in the Resend legacy path.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `email-pipeline`: Retried emails are actually delivered; digest email renders real content; every email exposes an unsubscribe path.

## Impact

- `internal/email/retry_worker.go`, `internal/email/pipeline.go`, `internal/email/smtp.go`, `internal/email/templates.go`, `internal/email/client.go`, `internal/llm/digest_worker.go`
- Tests: `internal/email/retry_worker_test.go`, `internal/email/client_test.go`, `internal/email/pipeline_test.go`, `internal/llm/digest_worker_test.go`
