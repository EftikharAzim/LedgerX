# LedgerX personal money tracking plan

Status: roadmap with an initial implementation; direct customer validation remains pending. Research checked on
2026-10-31 UTC. Target customer confirmed by the owner: personal money tracking.

The owner subsequently confirmed responsive web, budgets/reports, category
hierarchy and manually confirmed future expenses as priorities. See
[MONEY_MANAGEMENT_ROADMAP.md](MONEY_MANAGEMENT_ROADMAP.md) for the current
delivery scope, completed import/planning changes and remaining requirements.
Navigation now includes Budgets and interactive Reports; plans remain accessible
from Activity, Overview, Reports and secondary navigation. Category management,
monthly budgets and report behavior are documented in
[BUDGETS_REPORTS_CATEGORIES.md](BUDGETS_REPORTS_CATEGORIES.md).

## Product decision

Start with people who currently record personal spending in notes, spreadsheets,
or manual finance apps and find the upkeep tiring. They want to understand
everyday spending across cash, bank accounts, and wallets without having to adopt
a complete budgeting system. This narrower segment is a hypothesis to validate;
we have not interviewed these people yet.

Proposed promise:

> Understand your spending without connecting your bank or learning a budgeting system.

The reason to choose LedgerX should be a short, dependable routine: record a
purchase, see where money went, check the records against an account balance,
and leave. The customer benefit is less effort and uncertainty. The ledger
architecture supports that experience but is not the main customer message.

A modern interface, categories, exports, free software, and double-entry
accounting do not establish a unique advantage. Established alternatives already
offer many of them. We need evidence that our intended users find this routine
easier than their current method before claiming a reason to switch.

## What the research supports

This is qualitative desk research: official product documentation and selected
public user reports from three open-source finance products. Documentation shows
available alternatives; issue reports show specific frustrations. These are not
a representative customer survey, independently reproduced bugs, or proof of
how common each need is. GitHub feedback favors technical users. No interviews,
pricing study, or LedgerX retention study have been completed.

| Evidence | Customer need it suggests | LedgerX implication |
| --- | --- | --- |
| [Actual](https://github.com/actualbudget/actual#readme) describes free, open-source personal finance, synchronization, and local apps. [Money Manager Ex](https://github.com/moneymanagerex/moneymanagerex#features) lists categories, reports, imports, reminders, and budgeting. [Firefly III](https://github.com/firefly-iii/firefly-iii#readme) offers reports, imports, rules, and recurring transactions. | Customers already have capable alternatives. | Compete on a specific workflow and ease of use, rather than feature count or a free-software claim. |
| An [Actual mobile user](https://github.com/actualbudget/actual/issues/9159) reports losing their position after editing a transaction, making repeated categorization tedious. | Routine recording and correction must take little effort, including on a phone. | Preserve scroll, filters, and draft state; make categories easy to choose; test repeated tasks, not just the first transaction. |
| An [Actual user report](https://github.com/actualbudget/actual/issues/8932) describes pending and posted entries both affecting the balance; the user noticed through reconciliation. | People need to trust totals and find discrepancies. | Support starting balances, clear transfer/correction handling, import duplicate review, and reconciliation. Ledger consistency alone does not guarantee a match with the bank. |
| An [Actual import request](https://github.com/actualbudget/actual/issues/9065) describes extra navigation and the risk of importing into the wrong account. | Moving existing records should be easy and safe. | Show the destination account, currency, date interpretation, and preview before a CSV import is committed. |
| Money Manager Ex lists scheduled bill reminders; a [Firefly III user](https://github.com/firefly-iii/firefly-iii/issues/12987) requests control over recurring transaction timing. | Some users want less repeated entry and control over automation. | Validate reminders with LedgerX testers before building a scheduling system. One timing request does not establish broad demand. |

The central jobs to validate are:

1. "When I spend or receive money, help me record it quickly."
2. "When I review my week or month, show what I spent and where it went."
3. "When the balance looks wrong, help me find and correct the cause."
4. "When I move from another method, let me bring my records and take them out again."

## Where LedgerX stands

Available in the initial implementation: accounts with starting balances,
categorized income/expenses and explicit refunds, transfers, linked reversals,
spending by category, unified searchable Activity, and readable weekly, monthly,
yearly, custom, and all-time CSV exports. Navigation now uses Overview, Activity, Budgets and Reports. Safe category
organization, monthly budgets and interactive balance/category/trend reports
are available; see BUDGETS_REPORTS_CATEGORIES.md. Recurring schedules, explicit
payment confirmation and 30-day wallet projections are now available in
RECURRING_BILLS_AND_CASHFLOW.md. The expanded [customer requirements](./CUSTOMER_REQUIREMENTS.md)
record the evidence and release scope.

The most important gaps for this customer are:

- Guided CSV import and current posted-balance checks are implemented; historical
  statement clearing and automatic transfer matching remain later work.
- Guided editing and financial workspace backup/restore are implemented. Direct
  customer testing remains pending; the [test kit](./CUSTOMER_TEST_KIT.md) is ready.
- A Docker-only local package now serves the UI/API together, applies migrations,
  persists records/reports, and supports server data backup/restore. It still
  requires installation; no customer-facing hosted site exists.
- Saved-code recovery, password changes, and confirmed account deletion are
  implemented. Users must save a recovery code before losing their password;
  backup retention and storage-version cleanup remain deployment responsibilities.
- Start/import/restore onboarding, page code splitting, browser release checks,
  and a repeatable local load runner are implemented. Local measurements are
  not a production capacity guarantee or evidence of customer demand.
- Public hosting and external HTTPS access remain gaps; the owner has no hosting
  account. Verify zero-spend hosting against the whole persistent stack. Normal account rename/archive management remains unfinished. Category
  rename/archive, hierarchy and reviewed reassignment are implemented. See the
  [launch assessment](./LAUNCH_AND_SCALING.md),
  [baseline report](./SCALABILITY_BASELINE.md), and
  [deployment instructions](./RUNNING_AND_DEPLOYMENT.md).

Do not describe the current app as offline-first, automatically bank-synced, or
a complete budgeting product. It is a browser UI backed by an API. Record-derived
balances also depend on complete and correct user entries.

## Minimal experience

The initial release grouped daily work into these destinations:

- **Overview:** balance, income, spending, and a short category breakdown for the
  selected period and currency. Show uncategorized spending clearly. Offer one
  prominent **Add transaction** action.
- **Activity:** all transactions, search, compact filters, transaction details,
  and corrections. Account-specific history remains accessible here.
- **Reports:** the existing period exports, with readable CSV fields.

Planned activity is now directly accessible. The next navigation step adds
Budgets and integrates plans into Activity, as specified in
[MONEY_MANAGEMENT_ROADMAP.md](MONEY_MANAGEMENT_ROADMAP.md).

Keep accounts accessible from the overview and account picker. Keep category
management available during entry and in settings. Settings can hold currency,
appearance, and data controls. Avoid extra destinations for small features.

Entry should require only amount and account; make income/expense explicit,
default the date to today, and offer recent categories. Category and note can
remain optional. Keep transfers a clear, separate action. On mobile, retain the
user's position after reviewing or categorizing a transaction.

Preserve the existing visual direction: readable amounts, calm colors, clear
spacing, accessible contrast, and usable touch targets. Use a short category
list before adding more charts. Keep detailed ledger terminology inside
transaction details rather than the primary entry flow.

A proposed weekly review can make the experience recognizable: review
uncategorized entries, inspect spending by category, and optionally check an
account balance. It must not imply that all spending was captured simply
because the review is finished. This routine is a design hypothesis, not a
proven unique feature.

## Delivery order

These are ordered milestones, not calendar estimates. Deliver and test each
increment before expanding scope.

| Order | Outcome | Smallest useful scope | Acceptance criteria |
| --- | --- | --- | --- |
| 0 — validate | Confirm the problem and language. | Interview 5–8 people in the proposed segment; observe their current method; test a simple overview/activity prototype. | Record recent behaviors, repeated obstacles, and failed tasks. If upkeep or weekly spending clarity is not a recurring problem, revise the direction before extending the roadmap. |
| 1 — spending clarity | Answer "Where did my money go?" | Add an explicit starting balance and date; income/spending totals that exclude internal transfers and opening balances; category breakdown; an overview Add transaction action. | A transfer changes account balances without increasing income/spending. Opening balances do not become earnings. Corrections follow a documented period policy and do not double-count spending. Totals match category breakdowns, including uncategorized entries. Keep currencies separate and use explicit date/time-zone rules. |
| 2 — easy review | Find and fix records without losing context. | Unified Activity with search and account/date/category/type filters; category updates; guided correction of amount/date/account through linked reversal and replacement. | Filtering covers the full relevant history, not only the loaded page. Transfers appear once in the unified view. Returning from details preserves position and filters. Correction retries cannot reverse or replace twice. Show correction links without requiring accounting knowledge. |
| 3 — easy switching | Bring records in and check their accuracy. | CSV column mapping and preview; destination account/currency confirmation; invalid-row feedback; duplicate review; manual reconciliation. Add readable export amounts and category names alongside stable IDs/minor units. | Importing the same file again does not silently duplicate records. Legitimate identical purchases remain distinguishable through review. Failed imports have clear recovery behavior. Reconciliation shows the discrepancy and never silently changes a balance. Imports, corrections, category totals, and exports agree. |
| 4 — only if validated | Reduce repeated work. | Start with recurring reminders and a confirm-to-record action. Consider one simple monthly category limit only if pilot users need it. | Testers demonstrate the repeated task in their current routine and use the smaller solution. Reminders do not silently create transactions. Do not build reminders and a full budgeting engine together. |

For milestone 1, decide how corrections spanning periods are presented before
implementation. Test same-period and cross-period reversals explicitly. For all
period views, document one consistent rule for dates and time zones; the current
export service uses UTC boundaries. Do not quietly apply different boundaries to
overview totals and exports.

Before inviting general customers to store real records, verify backup and
restore, secure access, a plain explanation of data storage, and usable recovery
and data-deletion controls for the chosen deployment. A CSV report is not a full
backup. Choose one practical distribution method and test whether a
non-developer can start without instructions about Go, Node, or Docker.

## Free customer validation

Use the existing local app, a screen share, dummy records, and a spreadsheet for
notes. No paid research tool, bank integration, AI API, or hosting purchase is
needed for this stage. A hosted pilot would be a separate deployment decision;
free software does not establish free hosting forever.

Recruit people who already try to track their spending. Ask about actual recent
behavior, with permission to keep account details hidden:

- "Show me how you recorded your last few purchases."
- "When did you last stop tracking, and what made it difficult?"
- "Tell me about the last time your records did not match your balance."
- "What do you look at during a weekly or monthly review?"
- "How would you move your existing records into another app?"

Avoid using "Would you use this?" as evidence of demand. Observe whether people
can complete these tasks unaided: create an account with a starting balance,
record a categorized expense, find and correct it, explain a transfer's effect
on spending, and export a chosen period. Include a phone-sized screen.

Use the following as initial product targets, not industry benchmarks:

- At least 4 of 5 testers complete each core task without assistance.
- First account and transaction take under two minutes; a repeat entry takes
  under 15 seconds after familiarization. Observe whether these targets suit
  the chosen users rather than hiding failed attempts.
- All testers can explain why a transfer is excluded from spending.
- After a two-week pilot, at least 3 of 5 testers return for both weekly reviews
  without individual reminders and can name a useful decision the app helped
  them make. Record baseline behavior and reasons for drop-off.

These small samples are directional. If people do not return, investigate entry
effort and useful outcomes before adding more features. Improve the unclear
step and repeat the same task. Only claim a meaningful advantage after observing
it against the participant's current method.

## Defer

Defer paid bank connections, investment tracking, invoicing, business tax tools,
native apps, complex budgets, forecasts, extra appearance options, and an AI
assistant. Keep any existing optional categorization suggestions secondary to
manual choice. Add a deferred feature only when customer behavior shows that it
blocks the core routine and a smaller change will not solve the problem.

The implementation covers spending clarity, starting balances, refunds,
Activity filters and category review, readable exports, guided CSV imports,
reviewed transaction editing, financial workspace backups/restores, and current
posted-balance checks. See [IMPORTS_AND_BALANCE_CHECKS.md](./IMPORTS_AND_BALANCE_CHECKS.md)
for the switching workflow, recovery guarantees, and limits. The owner has now explicitly prioritized budgets, reports, category hierarchy
and future planning. Deliver these according to MONEY_MANAGEMENT_ROADMAP.md;
continue gathering evidence without blocking the authorized work on interviews.
This work does not establish product-market fit.
