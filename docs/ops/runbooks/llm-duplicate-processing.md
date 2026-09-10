# LLM Duplicate Processing & Dead-Letter Runbook

Scope: the `fix-llm-duplicate-processing` behavior — bounded per-comment
retries, backoff, dead-letter parking, partial-result acceptance, and the
provider-call telemetry that detects duplicate-work loops.

## Background

Before this change a failing comment batch was re-sent to the LLM provider on
every 10s tick with no attempt cap, no backoff, and a hardcoded 60s HTTP client
timeout that aborted slow-but-healthy generations. One 50-comment set was
observed sent 89 times (up to 4 HTTP attempts each). The fix caps attempts per
comment at `LLM_MAX_ATTEMPTS` (default 3), delays re-attempts with an
escalating backoff (`LLM_RETRY_BACKOFF`, default `1m,5m,15m`), parks
deterministic failures immediately, and persists valid partial results.

## Signals

### `event=batch_parked`

```
event=batch_parked channel_id=... job_id=... comment_ids=[...] reason=terminal|attempt_cap
```

- `reason=terminal`: HTTP 4xx (non-429), auth failure, or a response parse
  error. Parked immediately; no attempt budget consumed.
- `reason=attempt_cap`: the comments reached `LLM_MAX_ATTEMPTS` after transient
  failures (transport, 429, 5xx, timeout).

Parked comments are excluded from claiming until an operator un-parks them.
They are visible via the `noumena_llm_parked_comments` gauge.

### `event=batch_partial`

```
event=batch_partial channel_id=... job_id=... persisted=... missing_indexes=[...]
```

The provider returned a valid response but omitted or corrupted some comment
indexes. Valid results were persisted and billed; only the missing indexes
were re-queued. If you see this often, the model is truncating output — check
`LLM_BATCH_SIZE`.

### `noumena_llm_provider_calls_total{provider,retryable}` vs distinct jobs

This is the duplicate-work canary. Compute:

```
ratio = rate(noumena_llm_provider_calls_total[1h])
      / rate(noumena_llm_job_attempts_total[1h])
```

- **Alert threshold: ratio > 1.2 calls/job.** A healthy batch uses 1 provider
  call (plus occasional in-call retries). A sustained ratio ≫ 1.2 means the
  loop is back (retry storm, provider outage, or a regression in the attempt
  cap).
- `retryable="true"` attempts are transport/429/5xx retries; a spike there
  points at the provider, `retryable="false"` at deterministic failures.

### Other metrics

- `noumena_llm_job_attempts_total{provider,outcome}` with
  `outcome in completed|failed_transient|failed_terminal|parked`.
- `noumena_llm_parked_comments` — sampled per tick; the dead-letter depth.

## Triage

1. **Spike in `provider_calls_total` ratio** → check the provider status page,
   then `event=provider_failover` logs. Confirm `LLM_MAX_ATTEMPTS` is set as
   expected (default 3) and `LLM_BATCH_SIZE` is not back at the old 50.
2. **`event=batch_parked reason=terminal`** → read the job's `error_message`
   (`llm_processing_jobs`). A 401/400 means a config/credential problem: fix
   the key/model, then un-park (below).
3. **`event=batch_parked reason=attempt_cap`** → provider-wide outage. Verify
   failover engaged (consecutive-failure counter, `LLM_FAILOVER_ERROR_THRESHOLD`
   default 3). Once the provider recovers, un-park.
4. **`event=batch_partial` storms** → lower `LLM_BATCH_SIZE` (e.g. 10) and
   watch the ratio; the missing comments self-heal via the retry ladder.

## Un-park SQL

Reset the retry state so the next tick claims the comments again:

```sql
UPDATE comments
   SET llm_attempts = 0,
       llm_next_retry_at = NULL
 WHERE llm_attempts >= 3          -- the configured LLM_MAX_ATTEMPTS
   AND processed_at IS NULL;
```

To un-park a specific channel or comment set, add a predicate:

```sql
UPDATE comments
   SET llm_attempts = 0, llm_next_retry_at = NULL
 WHERE id IN ('<comment-id-1>', '<comment-id-2>');
```

To inspect what is parked:

```sql
SELECT platform_channel_id, COUNT(*)
  FROM comments
 WHERE llm_attempts >= 3 AND processed_at IS NULL
 GROUP BY platform_channel_id
 ORDER BY COUNT(*) DESC;
```

## Configuration

| Var | Default | Meaning |
| --- | --- | --- |
| `LLM_MAX_ATTEMPTS` | `3` | Max times a comment is sent for classification before parking |
| `LLM_RETRY_BACKOFF` | `1m,5m,15m` | CSV delays between re-attempts (indexed by attempt) |
| `LLM_BATCH_SIZE` | `15` | Comments per provider call |
| `LLM_PROCESSOR_BATCH_TIMEOUT` | `300s` | **Total** budget across all in-call attempts, not per attempt |
| `LLM_FAILOVER_ERROR_THRESHOLD` | `3` | Consecutive primary failures before permanent failover |
| `LLM_FAILOVER_WINDOW_SECONDS` | — | **Deprecated/ignored**; boot warning if set |

## Rollback

Redeploy the previous binary. The new columns (`comments.llm_attempts`,
`comments.llm_next_retry_at`) are nullable/defaulted and ignored by the old
binary, so no schema revert is needed.
