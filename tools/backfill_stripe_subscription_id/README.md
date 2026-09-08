# backfill_stripe_subscription_id

One-shot operator CLI that populates the new `users.stripe_subscription_id`
column for accounts that were upgraded **before** the column existed
(see OpenSpec change `backend-schema-and-env-fixes`).

## When to run

Once, immediately after deploying the schema migration that adds
`users.stripe_subscription_id`. After that point every new
checkout/subscription event populates the column natively via the
Stripe webhook handlers, so this tool becomes a no-op.

## Workflow

Always run dry-run first.

```bash
# 1. Dry-run (default). Prints the planned writes; modifies nothing.
DATABASE_PATH=./noumena.db STRIPE_SECRET_KEY=sk_live_... \
  go run ./tools/backfill_stripe_subscription_id

# 2. Review the output. Each line looks like:
#      user=<uuid> customer=cus_xxx subscription=sub_xxx
#    Lines ending in `subscription=<none>` are users whose Stripe
#    customer record has no live subscriptions — likely free-tier
#    drift; safe to leave unchanged.

# 3. If the planned writes look correct, re-run with --apply.
DATABASE_PATH=... STRIPE_SECRET_KEY=... \
  go run ./tools/backfill_stripe_subscription_id --apply

# 4. Re-run dry-run; output should be empty (or `0 candidates`).
```

## Summary line

The tool prints a final summary:

```
summary: 42 candidates, 38 backfilled, 4 had no Stripe subscription
```

- **candidates** — rows matching `stripe_customer_id IS NOT NULL AND
  stripe_subscription_id IS NULL`.
- **backfilled** — `--apply` UPDATEs that actually wrote a row (the
  defensive `WHERE stripe_subscription_id IS NULL` clause means a
  concurrent webhook write is *not* counted here).
- **had no Stripe subscription** — Stripe knows the customer but reports
  no `active` (and no historical) subscriptions; nothing to write.

`candidates - backfilled - had_no_subscription` is the count of rows
that hit a transient Stripe error during the run; re-run dry-run and
inspect those specific user IDs.

## Flags

| Flag           | Default                 | Purpose                                        |
| -------------- | ----------------------- | ---------------------------------------------- |
| `--apply`      | `false`                 | Actually write. Without this it's a dry-run.   |
| `--dry-run`    | `true`                  | Explicit; ignored when `--apply` is set.       |
| `--dsn`        | `$DATABASE_PATH`        | SQLite database path.                          |
| `--stripe-key` | `$STRIPE_SECRET_KEY`    | Stripe secret key.                             |

## Idempotency

Re-running `--apply` is a no-op:

1. The candidate `SELECT` filters out rows that already have the column
   set.
2. The `UPDATE` itself has a defensive `WHERE stripe_subscription_id
   IS NULL` clause, so a webhook firing concurrently with the backfill
   wins and the backfill skips.

## Resolution heuristic

For each candidate customer:

1. Stripe `subscriptions.list({customer, status: "active", limit: 1})`
   — if a hit, write that subscription ID.
2. Otherwise, `subscriptions.list({customer, status: "all", limit: 10})`
   sorted by `created` descending — pick the most recent. (This is the
   "best historical guess"; review dry-run output before applying.)
