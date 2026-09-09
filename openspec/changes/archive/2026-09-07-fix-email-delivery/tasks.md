## 1. Make retry worker actually deliver

- [x] 1.1 Persist rendered body (HTML/text) on email_log at enqueue
- [x] 1.2 `retry_worker` calls `Sender.Send` with persisted body on each retry
- [x] 1.3 Mark `sent` on success; `failed` after 3 attempts + metric/alert
- [x] 1.4 Test: pending row with a stub sender → send called, status flips to sent
- [x] 1.5 Test: send fails 3x → status failed

## 2. Fix idempotency race

- [x] 2.1 On UNIQUE conflict, read row status
- [x] 2.2 Dedupe only when `sent`; `pending` → continue; `failed` → log/re-send decision
- [x] 2.3 Test: first send fails (row pending) then same key re-enqueued → not dropped, eventually sent

## 3. SMTP per-command deadlines

- [x] 3.1 Set conn deadline before MAIL/RCPT/DATA/QUIT; clear after response
- [x] 3.2 Configurable `SMTP_CMD_TIMEOUT` (default 10s)
- [x] 3.3 Test: stub relay that stalls → command errors within bound

## 4. Render digest email from ContentJSON

- [x] 4.1 Parse ContentJSON into typed struct in digest_worker
- [x] 4.2 Populate TopIdeas/TopQuestions/SentimentDirection/SentimentChangePct + links in template data
- [x] 4.3 Render test: sample digest → non-empty lists, no `<no value>`

## 5. Unsubscribe support

- [x] 5.1 Add `List-Unsubscribe` header to digest emails
- [x] 5.2 Add signed unsubscribe link (reuse signer.go) → preferences opt-out
- [x] 5.3 Add footer link to marketing/digest templates
- [x] 5.4 Test: link verifies signature; flips digest opt-in off

## 6. Escape channel names

- [x] 6.1 `html.EscapeString` (or template render) for channelName in Resend legacy path
- [x] 6.2 Audit other fmt.Sprintf-HTML sites
- [x] 6.3 Test: channel name with `<script>` renders escaped

## 7. Verify

- [x] 7.1 `go vet`, `go build`, `go test ./internal/email/... ./internal/llm/...` green
- [ ] 7.2 Manual: transient SMTP failure → retry sends; digest email shows real content; unsubscribe works

> DEFERRED (archived 2026-09-07): 7.2 requires live SMTP relay + EMAIL_ENABLED=true staging env.
> Covered by unit/integration tests 1.4, 1.5, 2.3, 3.3, 4.3, 5.4, 6.3; full suite green.
