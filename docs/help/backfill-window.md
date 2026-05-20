# Backfill window

> Source of truth: [`specs/02-channels.md`](../../specs/02-channels.md).
> Related API: [`../api/channels.md`](../api/channels.md).

When you connect a YouTube channel to Noumena, we don't just start
ingesting comments going forward — we also **backfill** comments
that were posted to the channel in a recent window so your
dashboard isn't empty on day one.

## How long do we look back?

You can choose one of four backfill windows when you connect a
channel:

- **30 days** — fastest, smallest dataset.
- **60 days**
- **90 days** *(default)*
- **180 days** — slowest, largest dataset.

The default is 90 days. You can change the window for a tracked
channel any time via the dashboard or
`PATCH /api/channels/{id}/backfill-window`.

## Important: changing the window does not re-backfill

This is the single most common surprise:

> **Changing `backfill_days` on an already-tracked channel does NOT
> re-fetch additional history.** It only governs the **first time**
> a channel is added to Noumena.

Concretely:

- You connect Channel X with `backfill_days = 30`. We fetch the
  last 30 days of comments.
- Three days later you change `backfill_days` to `180`. We do
  **not** retroactively pull comments from days 31–180. The
  setting applies to **future first-time additions** of *other*
  channels.
- New comments posted from now on are ingested normally,
  regardless of the window setting.

The API's response makes this explicit:

```json
{
  "message": "backfill window updated",
  "days": 180,
  "note": "this setting affects future first-time channel additions only; it does not re-backfill this channel"
}
```

## Why this design?

YouTube's Data API is rate-limited and a backfill is expensive. If
changing the slider re-pulled history, an accidental nudge from 30
to 180 days could blow the daily quota for the entire fleet. Forcing
the choice up-front, once, keeps quota costs predictable and the
behaviour observable from the dashboard.

## What about the **shared** channel model?

Multiple Noumena users can track the same YouTube channel. When
**any** user connects a brand-new channel, the backfill they
configure runs against the shared store. Everyone tracking that
channel benefits from the data — but the same "no re-backfill on
change" rule applies: a later user whose preference is `180` does
not trigger an additional backfill if the channel was first added
with `30`.

## Common questions

- **"I want more history. What do I do?"** Disconnect the channel
  and reconnect it with the larger window. The shared model means
  that disconnecting only removes *your* tracker; the shared
  channel row may still hold less-than-180-days of comments if it
  was originally added with a smaller window.
- **"How do I see how much history I have?"** Use
  `GET /api/channels/{id}/status`. It returns
  `comment_count` and `last_synced_at`.

## Related

- [`../api/channels.md`](../api/channels.md) — full API reference.
- [`../ops/runbooks/channel-grace-window.md`](../ops/runbooks/channel-grace-window.md)
  — what happens when a shared channel loses its last tracker.
