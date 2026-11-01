# Personal finance product brief and delivery requirements

Updated 2026-11-01. This extends [PRODUCT_PLAN.md](PRODUCT_PLAN.md) and
[CUSTOMER_REQUIREMENTS.md](CUSTOMER_REQUIREMENTS.md). The owner's MoneyLover
description and import difficulties are direct requirements. They are not a
representative customer study. The authenticated MoneyLover web/Android UI has
not been independently inspected here; no new interviews have been conducted.

## Product promise to test

> Bring your history, understand today's money, and prepare for upcoming costs
> without losing control of what gets recorded.

Reasons to consider switching: dependable migration across years and wallets;
clear separation between recorded money and plans; reports that explain their
totals; quick entry and correction on a phone; readable exports and restorable
backups. These are benefits to demonstrate, not claims of superiority over an
established application. Budgets and recurring bills remain delivery work.

Responsive web comes first. The core manual workflow must work without a bank
connection, subscription, paid AI service, or new paid infrastructure. Local
Docker startup is available; there is still no public hosted service. Free
software does not establish unlimited free hosting or operating capacity.

## Keep the daily experience small

The target navigation is **Overview, Activity, Budgets, Reports**, with one
prominent Add transaction action. Account/settings and financial management
belong in secondary navigation. Currently **Planned** is directly accessible;
when Budgets ships, retain plans as an Activity view and dashboard shortcut.

Overview should answer three questions: what do I have, what did I spend, and
what is coming up? Keep wallet balances separated by currency. Show recent
activity and a compact spending comparison; reveal deeper charts on request.
Avoid adding a dashboard card for every feature. Never show invented financial
insights or paid-service configuration inside an ordinary spending flow.

## Delivery order and acceptance requirements

| Stage | Customer job | Required behavior | State |
| --- | --- | --- | --- |
| 1 | Bring six years of history | Detect separators/BOMs, review mappings and dates, route each source wallet/currency, review duplicates, commit atomically, keep receipts | Implemented; 50,000 records / 16 MiB per CSV, 100 visible rows per review page. Synthetic 6,753-row flow and local parsing of owner's file are checked separately. |
| 1 | Plan a future cost or income | Future CSV/manual entries create plans; plans affect no posted balances/reports; explicit actual-date confirmation records once; cancel without money movement | Implemented; pending list and confirmation/cancellation. Plan editing is still pending. |
| 1 | Keep report intent during migration | Preserve source IDs and Exclude Report; excluded entries still affect balance; flags/plans survive corrections, CSV export and financial backup/restore | Implemented. CSV is a posted-activity export; financial JSON backup also carries plans. |
| 2 | Organize categories safely | Rename, archive/unarchive, delete with explicit reassignment; two-level parent/subcategory tree; optional icon | Next. Never delete financial history as a side effect. |
| 2 | Review a period accurately | Previous/current/future month navigation; posted opening/ending balances; income and expense category drilldown; weekly/monthly/yearly/custom/all periods; compare equal periods | Period spending, Activity filters and CSV exports already exist. Opening/ending balances, comparison charts and income drilldown need implementation. Future views must label plans separately. |
| 3 | Set a realistic spending limit | Monthly overall/category budgets per currency, actual usage, remaining amount, near-limit/over-limit states and drilldown | Pending. Parent includes children exactly once; refunds reduce spending; transfers, opening money, reversed/replaced rows and report exclusions follow the spending rules. |
| 4 | Remember bills with less upkeep | Bill templates, recurrence rules, due/overdue view, next occurrences as plans; skip/pause/end series; prevent duplicate generation on retries | Pending. No automatic ledger posting. Test month-end dates, leap years, timezone changes and edits to one occurrence versus a series. Browser delivery must not imply reliable background notifications. |
| 5 | Track what I owe or lent | Named counterparty, principal, repayments, remaining amount, settlement history linked to wallet activity | Pending. Lending/repayment must not silently become ordinary income or expense. Reconcile reports and balances with explicit debt semantics before release. |
| 6 | Review a trip or occasion | Lightweight event/tag assignment and scoped totals/filtering | Pending; reuse Activity and Reports rather than adding another main tab. |

## Category deletion and hierarchy contract

Archive is the default for a category used in history: retain its name, reports,
budgets and transaction links; hide it from new-entry pickers. Unarchive restores
it. Imported categories should not be silently merged solely because their
labels look similar.

Deletion requires a visible impact preview: affected posted records, pending
plans, children and budgets. The user chooses another category of the correct
kind or **Uncategorized**. Move records atomically; never remove transactions or
change money. If children exist, explicitly move or archive them first. Preserve
an audit of reassignment so immutable receipts and old financial backups remain
understandable. Handle category references in historical corrections and import
duplicate comparisons consistently before allowing removal.

Use one parent and one child level first, for example Food → Groceries. Reject
cycles and cross-owner links. Parent reports aggregate direct parent entries and
all children once. Budget overlap must be explicit: a child budget can be a
sub-limit inside a parent, not an additional deduction from available money.

## Real-life scenarios that must drive design

| Situation | Requirement and boundary |
| --- | --- |
| Cash plus bank/mobile wallet | Fast wallet selection, clear transfers, starting money and balance checks. No duplicated expense when moving money. |
| Migrating a long history | One file can contain several wallets/currencies. No manual splitting for supported bounded files; retain exact source line references and avoid silent date/currency conversion. |
| Variable income | Income patterns may differ by month. Make budget assumptions editable; avoid an unconditional “safe to spend” claim. |
| Rent, annual insurance, future purchases | Keep commitments visible even after their due date. Plans never turn into expenses just because the clock advanced. |
| Refunds and mistakes | Refund the original spending category; linked correction history and reports must agree after edits, reversals and reimports. |
| Travel and foreign wallets | Separate balances/reports per currency until explicit exchange-rate/fee handling exists. Never sum unlike currencies. |
| Shared household | Defer collaboration until invitations, ownership, permissions and revocation are designed and tested. A shared password is not a sharing feature. |
| Privacy and switching away | Export without paywalls; include plans in backup, preview restore, preserve exact money and source links, and keep account deletion/recovery usable. |
| Small phone or assistive technology | Quick keyboard-operable entry; readable labels and focus; useful empty/error/retry states; no horizontal page overflow at 320px; paged large histories. |

## Automation and additional features

Use existing free history/keyword category suggestions first. Next useful
automation candidates are repeated-payee category rules, recurring-bill
suggestions and understandable spending comparisons. Explain why a suggestion
appears, allow dismissal, and require confirmation before changing money. No
paid language model is required for these rules and calculations.

Google Pay notification capture requires Android privileges and consent; a
responsive website cannot read those notifications. A future Android companion
needs a separate privacy/security design. Bank feeds similarly require provider
support and may have costs. Do not advertise either as implemented.

Google Sheets can receive the existing CSV export manually. Direct synchronization
needs an OAuth integration and conflict/revocation rules. ATM/bank finders are
lower priority than reliable daily tracking and can remain links to an external
map if research eventually supports them. Savings goals are a useful later
candidate; validate how reserved money interacts with budgets before adding a
second competing “available balance.”

## Evidence and release gates

Use synthetic data for browser screenshots, performance fixtures and committed
tests. The owner's uploaded statement is only parsed locally for compatibility;
it is not a test database seed or repository fixture. Keep customer interviews
and usability observations separate from automated checks; the interview count
remains zero.

For each stage, prove monetary invariants, ownership, retry safety, backup
compatibility and the relevant desktop/mobile/accessibility flows. Measure a
local workload before stating a capacity limit; one successful bulk import does
not establish concurrent hosted capacity. Review navigation and remove redundant
controls as the next stage ships. Public onboarding, HTTPS hosting and real
customer feedback remain launch gaps rather than completed product features.
