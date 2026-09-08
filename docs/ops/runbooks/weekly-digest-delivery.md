# Weekly Digest Delivery

Weekly cron cadence, idempotency, and retry semantics.

## Symptoms

- A user's weekly digest did not arrive.
- A digest is `deferred` or `failed`.

## Verification

- Cron runs Monday 08:00 UTC (`weekly_digests` rows for each user).
- Minimum 10 comments required — channels/users below that skip the digest.
- Delivery honors `digest_email_opt_in`; reruns are idempotent.

## Recovery

- **Deferred:** LLM failures retry 3 times over 2 hours, then the digest is
  marked `deferred`. Re-trigger by resetting the row:
  ```sql
  UPDATE weekly_digests SET status='pending' WHERE user_id=<uuid> AND status='deferred';
  ```
- **Email opt-out:** confirm `PATCH /api/user/preferences` shows
  `digest_email_opt_in=true`.
- **Email pipeline down:** see `EMAIL_ENABLED` kill-switch semantics in the
  root README; rows remain claimable when re-enabled.

## Postmortem

- Record whether failure was LLM-side or SMTP-side.
