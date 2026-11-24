# Account Deletion

Account deletion is handled by the GDPR runbook:

[`account-deletion-and-gdpr.md`](account-deletion-and-gdpr.md)

Covers the full flow: request → reversible window (`DELETION_REVERSIBLE_HOURS`,
default 720 h, 30 days) → `cancel_deletion` CLI → worker stages → hard delete.
