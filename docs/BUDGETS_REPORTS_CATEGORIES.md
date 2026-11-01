# Categories, monthly budgets and financial reports

These tools use the existing database without a paid provider, bank link or AI
model. Navigation is Overview, Activity, Budgets and Reports. Plans remain
reachable from Activity, Overview, Reports and secondary navigation; CSV downloads
remain available from Reports and `/exports`.

## Organize categories without losing money

At `/categories`, create or edit main categories and one level of subcategories.
Choose an icon and whether a category is for income, expenses/refunds, or both.
A new subcategory inherits its parent's type when no type is supplied. New-entry
choices respect type and archive status. The server rejects cycles, third
levels, foreign parents and incompatible parent/child types.

Rename changes the displayed label. Moving a subcategory changes its parent's
historical report and budget totals without changing money. A category with
children must remain a main category until its children are moved or deleted.

Archive keeps history and hides the category from new-entry choices. Archiving
a parent also archives its children. Unarchive the parent first, then each child
you want to use again. Existing entries, plans and budgets remain readable;
editing an entry can retain its existing archived category.

Deletion requires an impact review of entries, plans, children and budgets.
Choose another category or **Uncategorized**. The server rechecks the review
under the user ledger lock, reassigns transaction and plan labels atomically,
and clears suggestions pointing to the deleted category. Stale reviews return
409. Repeating a successful deletion with the same destination is safe.

Move or delete subcategories first. Move/remove budgets attached to the category
before deleting it; limits are never silently merged. Postings, balances, source
references and correction receipts are kept. Historical category identities,
deletion dates and reassignment links remain in financial backups so old receipts
can be restored. A deleted name can be reused for a new category.

## Monthly budgets

At `/budgets`, choose a month and currency, then set an overall, category or
subcategory spending limit. Limits are positive exact minor-unit amounts. Each
month/currency/category scope has one budget; at most 100 scopes are allowed per
month and currency. Editing or removing a limit does not change transactions.

Actual usage counts posted expenses and subtracts refunds in the month received.
It includes a parent's direct entries and children once. It excludes transfers,
opening money, report-excluded entries, reversed originals and their reversing
entries, and all unconfirmed plans. Changes to transactions, categories or the
hierarchy recalculate usage from the ledger.

Cards show usage, exact remaining/over amounts and textual within/near/over
states. Near means at least 80% used; over means spending exceeds the limit.
Refunds can produce negative usage and remaining money above the original limit.
The progress bar is clamped; the numeric amount is retained.

Overall, parent and child budgets overlap. Their remaining amounts are never
summed into one available balance. Limits do not move or reserve money. Automatic
rollovers, recurring budgets and scheduled notifications are not implemented.
**Use last month's budgets** is explicit: it copies active category and overall
limits while preserving destination limits, and can be retried safely.

## Interactive reports

At `/reports`, select weekly, monthly, yearly, custom or all-time reporting and a
currency. Monthly arrows and six selectable monthly trend points navigate
periods. Reports cover all normal wallets in that currency, including archived
wallets' history. Other currencies are never silently summed or converted.

Reports show posted opening/ending balances, income, net spending and net income;
category drilldowns for both income and spending; parent rollups; and a previous
period comparison. Activity links preserve dates/currency and include children
for a parent. CSV links preserve the selected period.

Opening balance is posted money before the inclusive start date; ending balance
includes posted money through the inclusive end date. All time starts at zero.
Balances include opening entries, transfers and report-excluded money. Income
and spending follow the corrected view used for budgets. The report discloses
and reconciles any difference:

`opening balance + net income + other balance changes = ending balance`

Other changes include starting money, excluded activity, and corrections whose
reversal occurs in another period. They are not silently treated as ordinary
income. Future plans remain separate, including when viewing future months.

Comparisons use the previous calendar month/year, previous week, or an equal
number of days before a custom range. The UI notes that a current period can be
incomplete. All time omits the comparison. Reads use one repeatable snapshot;
values and changes outside the supported exact range are rejected. Dates use
UTC boundaries, explicitly labeled in the interface.

## Backup and remaining scope

Financial JSON backups include hierarchy, icons, category types/archive/deletion
metadata, reassignment links and budgets. Restore remaps all related IDs,
validates hierarchy and monthly scope uniqueness, and commits atomically. Earlier
backups without these optional fields remain supported. A destination containing
budgets is not empty, even if it has no wallets yet.

Portable limits remain 32 MiB, 50,000 transactions, 150,000 postings and now up to
10,000 budget records. Historical category identities can be included in backup
category counts. Source CSV files and authentication credentials are excluded.

Recurring bills and cash-flow planning are now available in
RECURRING_BILLS_AND_CASHFLOW.md. Debts/loans, events, sharing, Android payment
capture, bank feeds and direct Google Sheets synchronization remain work in
[MONEY_MANAGEMENT_ROADMAP.md](MONEY_MANAGEMENT_ROADMAP.md). Public hosting and
customer validation remain launch gaps.

## Verification

Local validation passes 207 Go test cases with race detection, frontend tests,
Go/UI lint, production build and 55 browser/accessibility states. Coverage
includes ownership, cycles and inherited types; archive behavior; stale reviews;
deletion after corrections; reused names; refunds/exclusions/plans; budget copies;
parent Activity; balance reconciliation; comparisons; and backup ID remapping.
The 12 new financial-management states also check desktop/320px controls for
intersection, preventing overlapping inputs that accessibility rules alone can
miss. Browser fixtures contain invented records only.
