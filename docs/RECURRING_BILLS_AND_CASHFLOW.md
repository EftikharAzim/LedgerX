# Recurring bills, upcoming plans and projected cash flow

Updated 2026-11-02. This stage adds recurring expenses/income and a planning
estimate without a paid service, bank connection or language model.

## Daily workflow

Open **Planned activity** from Activity, Overview or secondary navigation.
**Recurring bills & income** opens schedule management at `/schedules`.
The main navigation remains Overview, Activity, Budgets and Reports.

Create a schedule with a name, active wallet, amount, income/expense type,
category, weekly/monthly/yearly frequency, first due date and optional end date.
A first date can be within the past year or in the future. Dates are UTC
calendar dates, consistent with existing reporting and imported date-only rows.

Occurrences are saved as pending plans. The application prepares 90 days ahead
on creation, then refreshes daily while running; a worker checks each minute.
Opening Planned activity also refreshes schedules. A first due date beyond the
90-day window is retained as a visible future plan. The app does not send
background browser notifications or automatically post payments.

Filter pending plans to **Overdue**, **Next 30 days** or all pending, with an
optional currency filter. Next 30 days includes today through day 29; overdue
means before today. Existing manual and imported plans remain available here.

## Record the actual payment

Use **Mark paid** or **Mark received** on a recurring occurrence. Review the
actual amount and calendar date, then confirm. Confirmation completes that
occurrence. The original estimate and actual recorded amount are both retained.
Partial-payment tracking is separate future work; confirmation does not infer
an unpaid remainder automatically.

The date must be no later than today and on/after the wallet's starting date,
when one exists. The amount must be nonzero, exact and in the same direction as
the original plan. A zero, reversed-sign or unsupported amount is rejected.
Confirmation creates one balanced ledger transaction; repeating an identical
confirmation returns its original ID. Changed actual details after completion
return a conflict instead of silently creating another payment.

Budgets and reports use the recorded amount, not the estimate. Plans have no
posted balance, spending or income effect until confirmation. Imported plans
retain their original source fingerprint/reference even when the actual amount
differs, so reimporting the estimate can still detect the completed record.

## Recurrence controls

| Control | Behavior |
| --- | --- |
| Skip occurrence | Cancel only this date. It remains skipped after refresh, resume or editing. |
| Pause | Put pending dates after today on hold. Already-due plans remain for payment or skipping. |
| Resume | Resume from today on the original calendar pattern. Dates missed during a pause stay canceled. |
| End | Cancel future occurrences. Already-due plans and all recorded history remain. Ending is terminal; create a new schedule to restart it. |
| Edit | Replace pending occurrences from today, preserving earlier due obligations, recorded history and explicitly skipped dates. Pattern changes require a first due date from today onward. |

Monthly recurrence stays anchored to the original day: January 31 → February
28/29 → March 31. Yearly February 29 becomes February 28 in non-leap years and
returns to February 29 in a leap year. No timezone, weekend or exchange-rate
conversion is inferred. Custom intervals, holiday adjustments, arbitrary
occurrence editing and calendar exports remain later work.

Edits and controls use revision checks. Category reassignment also updates the
schedule template and invalidates stale edits. Archived categories can remain
on existing schedules; new selections respect archive/type restrictions.

## Projected cash flow

Expand **Projected cash flow · next 30 days** in Planned activity. Each wallet
shows its recorded balance, pending money in/out, projected balance and a daily
timeline. Wallets and currencies are never silently combined or converted.

The estimate starts from the recorded wallet balance and applies pending plans
through day 29. Overdue plans are assumed to happen today. Same-day amounts are
netted; the view does not predict payment order within a day. Refunds in pending
plans are money in for cash-flow purposes, without changing their report type.

A projected daily balance below zero is shown even when expected later income
makes the ending estimate positive. The estimate depends on entered plans and
their assumed dates; it does not establish actual money available or account
for unentered obligations. Recording, skipping, pausing or ending recalculates
the projection. Paused/ended future occurrences contribute nothing; already-due
plans remain until resolved. Archived-wallet estimates remain readable, but a
wallet must be active to record payments.

## Integrity, backup and limits

Schedule writes and occurrence generation share the user's ledger lock.
Unique schedule/date pairs and durable creation/edit retries prevent duplicate
occurrences. The worker can run in multiple app instances safely. Each batch
refreshes up to 100 due users; this is a bounded workflow, not a hosted capacity
claim. At most 500 active/paused schedules, 10,000 saved schedules and 50,000
plans are supported per user.

Financial JSON backups include schedules, revision/anchor/generation metadata,
occurrence links, skip/pause/end reasons, estimates and actual amounts. Restore
remaps wallet/category/schedule/plan/transaction IDs and validates those links.
Retry credentials are excluded. Older backups remain readable. A populated
recurrence/actual-amount migration refuses a destructive downgrade.

## Verification

Local validation passes 215 Go test cases with race detection, frontend checks
and 66 browser/accessibility states, including 11 new recurrence states. Tests
cover month ends/leap years, concurrent creation and payment retries, skipped
dates, paused gaps, editing, stale revisions, ownership, category reassignment,
starting-date protection, changed actual amounts, per-wallet projections and
backup/restore ID remapping. Browser fixtures contain invented records only.

Debts/loans, events/tags, shared access, native payment capture, bank feeds and
direct Sheets synchronization remain in
[MONEY_MANAGEMENT_ROADMAP.md](MONEY_MANAGEMENT_ROADMAP.md). Public hosting and
direct customer validation remain launch gaps.
