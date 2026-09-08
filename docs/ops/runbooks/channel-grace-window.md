# Channel Grace Window

Shared-channel deletion grace behaviour for support staff.

## Symptoms

- A user "deletes" a channel but comments remain / channel is not immediately gone.
- A user expects immediate removal after unpausing/re-adding a channel.

## Verification

- `platform_channels.deletion_scheduled_at` set when tracker count drops to 0.
- A purge job removes the channel permanently unless a tracker reappears
  within `CHANNEL_DELETION_GRACE_HOURS` (default 24).

## Recovery

- Within the grace window, re-adding any user's tracker aborts the purge —
  shared comments are preserved.
- Outside the window the channel is purged and its comments removed; recovery
  requires restoring from the last backup
  (see [`backup-restore.md`](backup-restore.md)).

## Postmortem

- Note whether the user reconnected within the window and whether shared data
  survived.
