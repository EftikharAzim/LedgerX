# Bring your history and check your balance

Open **Import CSV** in the secondary navigation, Reports, or an account. These
tools use the existing database and worker. No bank connection, model, paid API,
or additional application dependency is required.

## Guided CSV import

1. Choose an active destination account and a UTF-8 CSV. You can create an account
   here. Files need one header row, at most 1,000 data records, 50 columns, and
   1 MiB. Comma, semicolon, and tab separators are supported. Download a sample
   from **Need an example?** if you need a starting point.
2. Match the columns. Check the source sample and explicitly choose the date and
   number format. Amounts can use one signed column or separate debit/credit
   columns. Map descriptions, categories, currency, transaction type, and bank
   references when available. Repeated header meanings stay unassigned.
3. Review rows, possible duplicates, and the projected balance. Invalid rows
   cannot be selected. Possible duplicates start unchecked; include an identical
   legitimate purchase individually after reviewing its match. **Select ready
   rows** never approves duplicates. Confirm the selection before importing.
4. Keep the receipt or open the imported Activity. Check the result against your
   bank or wallet's posted balance. Receipts and pending reviews can be reopened
   from **Recent imports and saved reviews**, including after a browser reload.

Only selected rows create transactions and categories. Unmapped or blank
categories stay uncategorized. Existing category names are matched without
case differences. Imported records do not enqueue optional model tasks.

Example for an account using USD:

```csv
Date,Amount,Description,Category,Currency,Type,Reference
2024-01-02,-12.34,Coffee,Food,USD,expense,BANK-001
2024-01-03,100.00,Salary,Income,USD,income,BANK-002
2024-01-04,4.00,Partial refund,Food,USD,refund,BANK-003
```

Map Type only when its values describe income, expense, or refund (debit,
withdrawal, credit, and deposit aliases are also accepted). An expense type
makes an amount negative. Income and refunds must be positive after applying
the chosen sign convention. Without Type, positive money is income and negative
money is expense, unless you explicitly choose the opposite sign convention.
For debit/credit files, use one nonzero side per row. Blank sides mean zero;
zero transactions and rows with both sides populated are rejected.

## Starting money and dates

The starting balance is the money present **before the imported history**, not
today's bank balance. Zero and money owed are allowed. Existing accounts retain
their starting component by default; empty accounts offer a starting balance.

If an account was created with today's starting date and you now want older
history, choose **Correct the starting balance or its date**. The preview shows
the old and proposed component. Confirmation reverses the old active starting
entry and creates its replacement, together with the selected import rows.
The original remains visible in the ledger. This does not discard other activity.

The optional start date uses UTC midnight. Blank uses the earliest valid date
in the file. Records preceding the chosen/existing start are invalid. Setting a
starting component requires at least one selected valid import row. Accounts
with several active starting entries require review before replacement.

ISO calendar dates and RFC 3339 timestamps are supported. Slash/hyphen dates
require an explicit day-first or month-first choice; two-digit years are not
accepted. Date-only records retain the source calendar date in Activity and
details, even in western time zones. Timestamps display locally. Reports use
UTC date boundaries throughout. Future records are excluded from this posted
activity import.

Decimal point (`1,234.56`) and decimal comma (`1.234,56`) formats are explicit.
Grouping must be valid; amounts use exact currency minor units and are never
rounded. JPY uses whole yen, KWD three decimals. Values and combined balances
must fit JavaScript's exact signed integer range in minor units. Each file
belongs to one account/currency; no exchange conversion is performed.

## Duplicates, interruptions, and recovery

The preview flags repeated source rows, repeated bank references, and existing
active same-date/same-amount transactions. A known distinct bank reference helps
distinguish otherwise identical purchases. These are possible matches, not
proof of duplication. Match details identify source lines or existing records.

All selected rows, starting corrections, categories, provenance, outbox events,
and the receipt commit in one database transaction. A failure rolls everything
back. Each preview ID and canonical row selection is a durable retry identity:
retrying that selection returns the same receipt. Changing the selection after
completion returns a conflict. The browser fetches the saved receipt if a save
response is lost. Reload the review/receipt if you are unsure whether it saved.

Financial writers serialize per user. If account activity changes after preview,
confirmation returns a conflict instead of using an outdated balance/duplicate
review. Edit and refresh the preview; after a reload you may need to choose the
file again. Pending reviews expire after 24 hours, with at most ten retained per
user. Expired previews are removed when another preview is created. Completed
receipts remain available; the recent list shows the latest ten sessions.

Raw CSV bytes are not persisted. A pending review stores its normalized rows
(including descriptions, categories, dates, amounts, and references), mapping,
and starting plan in the user's database. Confirmation clears that preview;
transactions, source references/fingerprints, date semantics, and receipt remain.
All import and balance-check endpoints require authentication and owner access.

## Current balance checks

Use **Check balance** on an account or import receipt. Enter the posted/settled
balance shown by the bank or wallet, including zero or a negative amount.
Pending card authorizations can explain a temporary mismatch.

LedgerX saves the actual balance, its recorded balance, the difference (actual
minus recorded), a ledger revision, date, and optional note. A mismatch points
you to Activity to review missing/duplicate records, refunds, and starting money.
The check never creates an adjustment or changes a transaction. Retrying the
same request key returns the same snapshot; new checks need a new identity.
Recent checks show when new ledger activity makes an earlier match stale.

This is a current posted-balance comparison. Historical statement closing dates,
per-line cleared/reconciled flags, bank sync, automatic transfer matching, and
full backup/restore are separate future work. Use the Transfer action for money
moving between your own accounts. Full LedgerX ledger reports are rejected as
imports because they include both transfer legs and correction history.

Exports retain their original columns and append `import_id,date_only`. Together
with the existing type/reversal columns, these explain origin and calendar-date
semantics. Ledger reports remain useful spreadsheet records, not restore files.

## Run and verify

Apply migration **000019** with `make migrate-up`, or restart `make dev`, which
applies pending migrations. Restart the UI with `make ui-dev` from the repository
root. Existing data is retained. The down migration refuses to discard populated
import provenance or balance checks.

Parser/unit tests, PostgreSQL/Redis race integration tests, and browser checks
cover normalization, ownership, duplicate opt-in, starting corrections,
concurrent retries, stale previews, injected-failure rollback, receipts,
backdated balances, category/spending/export metadata, and comparison snapshots.
See [UI_UX_REVIEW.md](./UI_UX_REVIEW.md) for the optional browser runner.

Verification on 2026-11-01 passed the full Go race suite with dedicated
PostgreSQL/Redis, Go lint, eight UI test files, UI lint, production build, and the
API smoke test. The new import browser run passed ten axe states; the existing
UI/UX regression passed thirteen more, with zero reported violations. A real
background-worker CSV was downloaded and parsed. Migration 19's populated-data
rollback guard and empty rollback path passed transactional database checks.
The existing main JavaScript bundle size warning remains. These checks do not
replace representative customer or screen-reader testing.
