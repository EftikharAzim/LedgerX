# Personal spending: research and actionable requirements

Research date: 2026-10-31 UTC. This extends [PRODUCT_PLAN.md](./PRODUCT_PLAN.md).
The owner is conducting direct customer conversations separately. The initial
release implements the Now requirements below, including guided CSV import and
current balance comparisons, reviewed corrections, and financial workspace backups. Historical statement clearing, reminders, and other
later requirements remain on the roadmap.

## Scope and evidence quality

Reviewed public documentation for Actual Budget, Money Manager Ex (MMEX),
Firefly III, Ivy Wallet, and Cashew, plus individual user reports. The sources
cover desktop, mobile, browser, manual entry, importing, self-hosting, and more
advanced budgeting workflows. Ivy Wallet's README states that it is no longer
maintained; its older reports provide historical scenario evidence only.

This environment permits GitHub access but does not provide general web search
or access to consumer-review sites. This is a broad qualitative review within
that restriction, not comprehensive market coverage. Open-source issue authors
are a self-selected, relatively technical sample. Reported bugs were not
independently reproduced, and some issues have since closed. A closed issue
does not make the underlying customer need disappear, nor prove that the current
product still has the reported problem. No prevalence, market-size, willingness
to pay, or retention claims follow from this sample.

## Real-life scenarios and resulting requirements

| Perspective / scenario | Source evidence | Requirement | Priority |
| --- | --- | --- | --- |
| A beginner starts with money already in a bank account. | [MMEX #8370](https://github.com/moneymanagerex/moneymanagerex/issues/8370) describes confusion when the initial balance is missing from reconciliation calculations. | R1: Capture an explicit starting balance and date. It affects balance but never income/spending. Account and starting entry must save together; retries must not create duplicates. | Now |
| A person moves salary from checking to savings or withdraws cash. | [MMEX #8581](https://github.com/moneymanagerex/moneymanagerex/issues/8581) reports transfers appearing as expenses and confusing sinking-fund totals. | R2: Distinguish spending from transfers. Present a transfer once in combined Activity with both accounts, while preserving both ledger postings. | Now |
| Someone returns part of a clothing purchase. | [Actual #8636](https://github.com/actualbudget/actual/issues/8636) describes refunds counted as income instead of reducing category spending. | R3: Offer an explicit refund type. A refund reduces spending in its chosen category when received; it does not become salary/income. Allow net refunds in a period rather than clamping totals to zero. | Now |
| A busy mobile user categorizes several purchases in a row. | [Actual #9159](https://github.com/actualbudget/actual/issues/9159) describes repeated scrolling after each edit. [Cashew's documentation](https://github.com/jameskokoska/Cashew#readme) lists search and filters as basic transaction tools. | R4: Search the whole history with date, account, category, and type filters. Retain filters and position when opening details and changing a category. | Now |
| A person reviewing the week wants an answer, not an accounting lesson. | [Ivy's README](https://github.com/Ivy-Apps/ivy-wallet#readme) explicitly asks how much money a person has and what they spent this month. Firefly's [README](https://github.com/firefly-iii/firefly-iii#readme) emphasizes insight and control. | R5: Show balance, income, net spending, and a short category breakdown. Make uncategorized spending visible. Provide Add transaction directly from Overview and Activity. | Now |
| Someone corrects an entry made in an earlier month. | [Actual #8932](https://github.com/actualbudget/actual/issues/8932) describes a balance discrepancy discovered through reconciliation. LedgerX already preserves corrections through reversals. | R6: Explain correction semantics. The new spending view excludes reversed originals and their reversals, restating the original period. Keep every posting in account history and raw CSV. Never count a reversal as fresh earnings. | Now |
| A traveler or migrant has accounts in different currencies. | [Cashew #1169](https://github.com/jameskokoska/Cashew/issues/1169) describes fetched exchange rates that differ from the user's actual rate. | R7: Keep currencies separate until explicit conversion accounting exists. Preserve currency precision, including JPY whole units. Do not show a misleading combined total. | Now: preserve and test; FX deferred |
| A keyboard or screen-reader user reviews purchases. | [Actual #2353](https://github.com/actualbudget/actual/issues/2353) details focus, labels, contrast, and zoom barriers. | R8: Use labeled controls, visible focus, dialog focus return, meaningful text alongside color, and layouts usable on a phone and at increased zoom. Test the new flows by keyboard. | Now: new flows |
| A person checks records near midnight or a month boundary. | [Ivy #3434](https://github.com/Ivy-Apps/ivy-wallet/issues/3434) reports a six-hour time shift and confusing balance updates. | R9: Convert entry times explicitly and use consistent report boundaries. Retain the existing UTC report convention and label it; display transaction timestamps in the user's local zone. | Now |
| A spreadsheet user wants records they can understand outside the app. | MMEX [features](https://github.com/moneymanagerex/moneymanagerex#features) and Cashew's README offer data import/export. [MMEX #8071](https://github.com/moneymanagerex/moneymanagerex/issues/8071) describes ambiguous import labels. | R10: Add readable decimal amounts, category names, and transaction types to exports while keeping stable IDs and minor units. Include CSV mapping/preview with unique labels, duplicate review, exact money, and safe retry behavior. | Implemented |
| A person compares records with a bank or wallet. | [Actual #8932](https://github.com/actualbudget/actual/issues/8932) and [MMEX #8370](https://github.com/moneymanagerex/moneymanagerex/issues/8370) describe balance discrepancies and starting money. | R15: Save a current posted-balance comparison and signed discrepancy. Never silently adjust records. Mark earlier matches stale after new activity; support safe retries and direct Activity review. Historical statement clearing remains later work. | Implemented |
| A person checks balances in public on a phone. | [Actual #9237](https://github.com/actualbudget/actual/issues/9237) requests device-specific privacy mode. | R11: Validate a per-device hide-amounts option. Distinguish screen privacy from encryption, storage privacy, and access control. | Next, after interviews |
| A household member with irregular income worries about bills before payday. | [Cashew #1291](https://github.com/jameskokoska/Cashew/issues/1291) reports an incomplete upcoming balance when recurring occurrences are omitted. [Firefly #12987](https://github.com/firefly-iii/firefly-iii/issues/12987) requests control of recurrence timing. | R12: Validate reminders and a simple monthly category limit first. Never label the current balance “safe to spend” without obligations and timing information. Do not mix forecast amounts with recorded balances. | Later, validate first |
| A person wants a calmer everyday interface. | Owner request in this session; changes are a design hypothesis until customer testing. | R13: Compact headings, on-demand filters, conditional review prompts, dismissible confirmations, and UI/UX checks covering keyboard, contrast, and phone layouts. | Now |
| A person repeatedly categorizes the same purchases. | Owner request for useful AI features, combined with the existing requirement to reduce categorization effort. | R14: Provide optional, explainable category hints from confirmed personal history with a free fallback. Require acceptance and preserve manual choices. Model-backed work remains optional and separately evaluated. | Now |
| A person corrects a mistake or needs to recover their records. | Owner-approved implementation plan and the roadmap’s recovery requirement. | R16: Review edits with before/after balances; preserve original/replacement links and safe retries. Export a private versioned financial workspace backup and restore atomically into an empty workspace, preserving imports, corrections, checks, and appearance. | Implemented |
| Potential customers try the full routine. | Owner is gathering direct customer feedback; no sessions have been reported yet. | R17: Prepare six observed tasks for five people, dummy data, an observation sheet, and an honest results summary. Separate automated scenarios from human evidence. | Kit implemented; customer sessions pending |

Additional sources reviewed: [Actual's project documentation](https://github.com/actualbudget/actual#readme),
[Actual's import-account request](https://github.com/actualbudget/actual/issues/9065),
and the competing product READMEs linked above. Documentation establishes what
alternatives advertise; it does not establish that customers are satisfied.

## Gaps and switching proposition

The evidence does not prove a feature missing from every competitor. Several
competitors already have more capabilities than LedgerX. The opportunity to
test is the complete routine for people who want manual spending clarity with
less setup and upkeep. The relevant alternative can be a spreadsheet, not only
another finance application.

Proposed customer promise:

> Know what you spent, where it went, and what needs checking—without connecting your bank.

Three reasons to try it, which implementation must substantiate:

1. Record a purchase or refund directly from the main screen.
2. Get spending totals that treat transfers, starting money, and corrections
   consistently, with a short explanation of the numbers.
3. Find a record quickly and take readable records away whenever needed.

These are intended benefits, not proven superiority. Do not promise automatic
capture, offline operation, bank-grade security, complete financial advice, or
free hosted service. Bank linking is not required for LedgerX's manual workflow.

## Experience and release decisions

Keep three primary destinations: **Overview, Activity, Reports**. Accounts remain
available from Overview and Activity; categories remain available during entry
and through a secondary management link. Keep one prominent Add transaction
action. Retain the current calm visual direction and accessible dialogs.

Implement R1–R6 and R10's readable exports as the first release, with R7–R9 as
quality requirements. The release should work as a complete manual routine,
without a paid dependency. Guided CSV import and current balance comparisons
now include preview, matching, duplicate review, and recovery. See
[IMPORTS_AND_BALANCE_CHECKS.md](./IMPORTS_AND_BALANCE_CHECKS.md). Recurring bills,
budgets, shared households, historical statement clearing, and currency
conversion remain separate decisions. This is prioritization, not evidence that
those scenarios are unimportant.

Acceptance checks must include starting money, salary, an expense, a partial
refund, a transfer, an uncategorized purchase, same/cross-period reversals,
multiple currencies, UTC boundaries, filtering beyond the first page, ownership,
safe retries, readable CSV, and phone/keyboard operation.

## How customer conversations change the requirements

Ask people to demonstrate recent behavior: recording a purchase, finding an
older expense, handling a return, checking a balance discrepancy, and reviewing
a week. Ask what would stop them switching, including setup, migration, trust,
and mobile effort. Do not interpret feature requests or “I would use it” as
proof of demand.

Track evidence for each requirement: participant's current method, recent
problem, workaround, frequency, impact, and observed task result. Keep personal
financial details out of research notes unless the participant explicitly wants
to share them. Test initial targets from PRODUCT_PLAN.md; compare task effort
with each participant's existing method. Use two weeks of actual return behavior
to assess whether the weekly routine helps. Revise the segment or promise if
people cannot name a useful outcome or do not return.

## Initial release verification

The implementation passed the Go race suite against dedicated PostgreSQL/Redis
test services, including repeated concurrent creation retries, same/cross-period
reversals, refund-only periods, owner isolation, UTC boundaries, and pagination.
UI tests, Go/UI lint, and the production UI build passed. The build retains the
existing warning about the main JavaScript bundle size.

Chromium checks covered lost-response account replay, categorized income and
expenses, partial refunds, currency separation, whole-history search, transfer
destination filters, category updates, pagination, preserved scroll and keyboard
focus, account management, and 320/390-pixel phone layouts. A real background
worker CSV was downloaded and parsed, verifying readable amounts and categories
alongside the underlying ledger metadata. The existing API smoke test passed.
These checks establish implementation behavior, not customer demand or usability
success with representative participants.
