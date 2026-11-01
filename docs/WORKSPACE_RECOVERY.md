# Transaction editing and workspace recovery

These tools complete the manual spending workflow without a bank connection,
model call, paid service, or new application dependency. Customer testing is
prepared in [CUSTOMER_TEST_KIT.md](./CUSTOMER_TEST_KIT.md).

## Edit a transaction

Open a record in Activity or an account and choose **Edit transaction**. Change
the amount, date, account, note, category, or income/expense/refund type. Choosing
Uncategorized removes a category. Transfers expose their source and destination;
starting entries allow a negative amount or zero. Currency remains fixed, and
the selected accounts must be active and use the same currency.

Choose **Review correction** to see changed fields and every affected account's
before/after balance. Confirm the review, then save. A canceled original or a
reversal cannot be edited; follow its replacement link or use the dedicated
reversal action to cancel an active record. Unsupported legacy multi-leg records
retain their dedicated reversal action.

Saving commits the balancing reversal, replacement, correction links, outbox
events, and receipt in one database transaction. If any step fails, the original
remains active and no partial correction survives. A zero starting component is
represented by reversing the old opening entry and retaining its tracking date,
without inserting a zero posting. Starting dates must precede active activity;
each account retains at most one active starting component in this edit flow.

Preview reads the original under the same per-user ledger lock as writes. Its
token covers the source and affected account states. A changed source/category
or account returns a conflict and requires a refreshed review. Category changes
also use this lock. The same write key/body replays its receipt, including after
a lost response; the browser can recover the receipt using that private key.

Spending excludes the canceled original and reversal, then uses the replacement
in its chosen period. Account balances and ledger reports retain every posting.
Activity shows original/replacement links. Existing bank-import provenance follows
correction chains, so a corrected record retains its import receipt and bank
reference for duplicate review. Date-only values keep their UTC calendar date;
timestamps display locally. Unchanged timestamps retain their original seconds
and fractional precision. Currency formatting also retains exact minor units
near the supported JavaScript integer boundary.

## Save a workspace backup

Open **Reports → Backup & restore**, or use the secondary desktop navigation.
Choose **Download workspace backup**. Keep the `.ledgerx.json` file private.
It is a plain JSON financial backup, not an encrypted archive.

The backup contains:

- All accounts, currencies, active flags, tracking dates, and categories, including
  hierarchy, types, icons, archive/deletion history and reassignment links.
- Monthly budgets, including their original scopes and limits.
- Every transaction and posting, including transfers, refunds, starting entries,
  canceled originals, and reversals, with their original dates and notes.
- Completed import receipts, source fingerprints/references and date semantics.
- Edit history and linked correction receipts.
- Saved bank/wallet comparisons and their position in ledger history.
- Optional account icons/colors from the current browser.

It excludes authentication credentials/tokens, HTTP retry keys, unsaved previews,
server configuration, cache/snapshots, generated report files, and background
jobs. Reports and snapshots can be regenerated from restored authoritative
records. This is a complete financial workspace backup, not a server/database
or authentication backup. CSV reports remain separate spreadsheet documents.

One repeatable-read snapshot supplies the export. The `ledgerx.workspace`
version-1 envelope has a SHA-256 checksum over canonical JSON (with exact JSON
numbers). That detects accidental content changes; it is not authentication or
encryption. Unknown versions, missing references, invalid currencies/amounts,
unbalanced postings, incorrect reversals, inconsistent receipts, and forged
historical balance checks are rejected before restore.

## Restore and recover

1. Choose the original JSON file under **Backup & restore**, then **Preview
   restore**. Review its date, account balances, currencies, and record counts.
2. Restore requires an empty financial workspace. A current workspace with
   records can preview a file. **Sign out and create a new workspace** takes you
   through an explicit session switch; after registration, the app returns to
   the recovery tools. Choose the same file there. Your old workspace remains
   accessible using its own login. An existing empty login can also be used.
3. Confirm the review and choose **Restore workspace**. All financial rows,
   relationships, and the completion receipt commit together. No records are
   merged into or overwritten in a populated workspace.
4. Review the restored Activity or Overview. The receipt is safe to reload;
   optional account appearance is reapplied from the receipt. Generate reports
   again as needed.

Restoring allocates fresh IDs and remaps every relationship, including receipt
links and balance-check posting revisions. Original occurred/creation dates are
preserved. The empty login's default system account is replaced transactionally,
so old snapshots cannot hide restored historical postings. Stale snapshot jobs
for that removed empty account end without repeated failures. A new restore
emits one workspace event and does not re-enqueue optional categorization work.

The review ID and checksum form a durable retry identity. If a response is lost,
the browser fetches the stored receipt; retrying the same review cannot duplicate
its records. Competing reviews serialize, and only the first can fill an empty
workspace. Failed restores roll back all financial rows and remain retryable.
Private reviews expire after 24 hours; at most three pending payloads are retained
per user. The next preview prunes expired/older pending payloads. Completion
clears the staged file; receipts remain owner-scoped and available for recovery.

Financial backups include planned activity (pending, posted and canceled) and
report-exclusion flags. Imported plans retain source links; confirmation links
are remapped during restore. Retry keys are excluded. Older backups without
these optional fields remain supported. Plans are bounded at 50,000 per backup.

Format limits: 32 MiB, 50,000 transactions, 150,000 postings, 1,000 accounts,
10,000 categories/import sessions, and 50,000 saved checks. Financial amounts and
account balances must fit the supported exact signed JavaScript integer range
in currency minor units. The file is validated again during commit.

## Development and verification

Pull `codex/modern-ui`, restart `make dev` (applies migration **000020**), and run
`make ui-dev` in another terminal from the repository root. Open
`http://localhost:5173`. Existing records remain available. The down migration
refuses to discard saved correction history or completed restore receipts.

Go race tests exercise concurrent edit/restore replay, stale review rejection,
category races, injected-failure rollback, ownership, corrupt files, historical
checks, snapshot-safe restores, and relational round trips across currencies,
imports, and corrections. UI tests cover exact money/time handling. The optional
browser runner covers five synthetic workflows, phone/desktop accessibility,
actual signup/session switching, lost responses, restored appearance, and an
actual worker CSV. See [UI_UX_REVIEW.md](./UI_UX_REVIEW.md). These automated checks
are separate from customer observation; the customer sheet starts unobserved.

Release verification on 2026-11-01 passed the full Go race suite, Go/UI lint,
nine UI test files, three customer-summary tests, production build, API smoke,
and all three browser suites. The suites reported zero automated accessibility
violations across 33 states. Database checks include a 1,000-record restore,
migration rollback guards, moved-import duplicate review, and rejecting a staged
file changed after review. The existing main-bundle size warning remains.

Category organization and monthly budgets are now included in the same portable
format. Restore remaps parent/reassignment/category/budget IDs, validates unique
monthly scopes and the two-level hierarchy, and refuses a destination containing
budgets. Up to 10,000 budget records are portable; the per-month/currency bound is
100. See BUDGETS_REPORTS_CATEGORIES.md for the full behavior.

Recurring schedules and their occurrence/revision/date links, cancellation
reasons and actual recorded amounts are included in financial backups. Restore
remaps those IDs and preserves skipped and resolved occurrences so refresh
cannot recreate them. Creation/edit retry credentials are excluded. Earlier
backups without these optional fields remain supported. The portable schedule
bound is 10,000, with at most 500 active/paused schedules.
