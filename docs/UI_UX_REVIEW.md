# UI, UX, and helpful automation

Updated 2026-11-02. Product standards for future work are saved in
[AGENTS.md](../AGENTS.md).

The owner requested a calmer interface, UI/UX checking, and useful AI features.
This review extends [CUSTOMER_REQUIREMENTS.md](./CUSTOMER_REQUIREMENTS.md).

The packaged release runner covers everyday tracking, imports, budgets/reports,
recurring plans, recovery, security and product layout. The current local run
passes 82 browser/accessibility states with zero reported violations, including
16 new product checks. Frontend tests, lint and production build pass. See
[release verification](./RUNNING_AND_DEPLOYMENT.md#release-checks). These are
automated observations, not direct customer research.

## Product editing pass — 2026-11-02

A review of the running app found repetitive slogans, decorative export/login
illustrations, secondary tools hidden on phones, and ordinary BDT amounts
breaking mid-decimal at 390px. This pass makes the following changes:

- Plain task headings across login, account creation, budgets, reports, CSV,
  categories and onboarding. Remove repeated motivational/status filler.
- Remove the static workspace avatar, decorative document panels, login artwork
  and repeated footer prompts. Delete their unused CSS rules.
- Add a More page grouping planning, management, data and account tools. All nine
  links work by keyboard at phone widths as well as desktop; existing routes
  stay valid.
- Show the overview summary before period controls. Hide the currency chooser
  when only one currency exists. Keep date/accounting explanations accessible
  under the existing disclosure. Native month/date controls retain enough width
  for their text and calendar button.
- Fit full monetary values to their available width, without rounding, clipping
  or splitting digits. Use wider summary cards on narrow screens. Sum account
  balances with integer arithmetic and show an explicit unsupported-total state
  instead of silently rounding an aggregate outside the exact range.
- Keep export period selection, queue/ready/error states and download behavior.
  Move format details into a disclosure and show request history only after a
  request exists. Preserve import, correction, restore and deletion safeguards.

`product_ui_check.cjs` covers realistic BDT amounts, USD/JPY, the exact supported
limit, unsupported aggregate handling, navigation at 320/390/640/1440px,
keyboard access and CSV generation/download. It uses an owned synthetic account
and removes it afterward. This complements the existing financial workflow
checks; it does not replace direct customer observation.

The expanded ready-state export check also found low-contrast request metadata;
the metadata now uses readable secondary text and larger labels. The generated
CSV is downloaded and checked for the selected synthetic records.

## Experience changes

- Keep Overview, Activity, Budgets and Reports as the core destinations. More
  provides the same grouped management/tools menu on phones and desktop. Keep
  one prominent Add transaction action.
- Use compact everyday headings. Remove repeated breadcrumbs/date headers,
  decorative sidebar messages, and the large motivational review card.
- Show a small review prompt only when spending records need categories.
- Keep Activity search visible. Put account/category/type/date controls behind
  a Filters button, show the active filter count, and preserve filter values
  when collapsed. Links with active filters open the controls initially.
- Default transaction date/time to now and expose editing through a disclosure.
  Keep amount, account, note, and category in the normal entry flow.
- Allow success confirmations to be dismissed. Keep errors and recovery actions
  visible, and never delay recording while an optional hint is being fetched.

These changes reduce visible controls and repeated copy. Whether customers find
the result easier remains a question for observed task testing, not an automatic
consequence of a cleaner screenshot.

## Free smart category hints

The entry form offers **Suggest category** when the user has categories and a
short note. It runs only when requested; it does not call a model while typing.

1. Check up to 20 recent, confirmed entries with the same normalized note,
   currency, and transaction kind. Case and repeated whitespace are normalized.
2. Exclude unconfirmed suggestions, reversed/canceled records, starting balances,
   transfers, and every other user's records.
3. Suggest the leading category when at least 80% of those matches agree. With
   one matching record, explain that there is only one prior choice. The 80%
   threshold is a conservative heuristic, not a calibrated accuracy estimate.
4. When there is no matching history, try a unique whole-word category-name
   match. If history conflicts or several category names fit, return no choice.
5. Explain the source and require **Use [category]** before changing the draft.
   Changing note, currency, or kind hides an earlier result. A manual selection
   is never overwritten, and no transaction is created until Record is pressed.

This helper uses deterministic history matching and rules. It is free to run
with the existing API/database and requires no external model or paid API key.
It reads data but never writes ledger amounts, categories, or model suggestions.
The existing optional Claude-backed background categorizer remains separate;
its keyword backend also runs without a paid API key. See
[AI_CATEGORIZATION.md](./AI_CATEGORIZATION.md).

Potential later AI work is limited to tasks customers demonstrate a need for:
plain-language transaction drafts, receipt extraction, or a short explanation of
an existing spending breakdown. Evaluate accuracy, cost, local-model options,
and review effort before implementation. Amounts and summaries must continue
to come from the ledger, and any extracted transaction requires confirmation.
Keep assistance inside the relevant task rather than adding a permanent chat
panel or interrupting people with financial recommendations.

## UI/UX checks for a release

| Area | Check |
| --- | --- |
| Visual hierarchy | Balances and records precede promotional content; one primary action; optional controls stay out of the way. |
| Phone layout | Check 320px and 390px widths, plus 640px reflow. No horizontal overflow or clipped form controls. |
| Keyboard | Reach search, Filters, disclosures, category actions, and Record with the keyboard. Trap focus in open dialogs and restore it on close. |
| Accessibility | Run axe-core for WCAG 2 A/AA and 2.1 A/AA checks on authentication, Overview, Activity with filters open/closed, entry dialogs, hints, and Reports. Review labels, text contrast, and status announcements. |
| Feedback | Confirm loading, empty, failure, retry, and successful states. A hint failure does not block manual entry. |
| Hint correctness | A result explains its source, abstains on ambiguity, follows the current draft, requires acceptance, and never affects amounts. |
| Real tasks | Record and categorize a purchase, find an older entry, correct a category, change the reporting period, and export records. |
| Customer observation | Watch potential customers complete tasks unaided. Record time, errors, hesitation, and distractions; compare with their existing method. |

Automated accessibility checks do not establish full WCAG compliance or replace
screen-reader and representative customer testing. Financial regression tests
remain separate from visual checks.

## Verification

The release passed 13 axe-core audit states for the WCAG tags above, with zero
reported violations in the tested states after correcting secondary-text
contrast. Desktop and 320/390/640px layout checks passed. At 390px, the first
Activity record appeared within the first 650px of the page with filters closed.
This is a product check target, not an industry usability benchmark.

Browser checks passed for keyboard filter toggles, dialog focus/return, editable
date disclosure, explicit hint acceptance, stale hints after note/type/currency
changes, free keyword fallback, nonblocking hint failures, and dismissible
confirmations. A new entry now defaults to the first active account in the
displayed currency. Backend race tests and Go lint passed; UI tests, lint, and
the production build passed. The existing main-bundle size warning remains.

Direct customer usability testing remains pending.

Reviewed editing and workspace recovery add ten more zero-violation axe states:
edit forms, before/after correction reviews, backup tools, nonempty-workspace
protection, restore review, and restored receipts at desktop/320/390px widths.
The five synthetic workflows cover corrections with lost responses and stale
reviews, transfer accounting, refunds and JPY, imported dates/references, and
recovery through actual sign-out/signup. A worker CSV is downloaded and parsed
after restore. These are automated regressions with zero customer participants.

The [customer kit](./CUSTOMER_TEST_KIT.md) contains six observed tasks for five
people, dummy data, a blank result sheet, and a free Python summary tool. Empty
observations remain unobserved; no customer results are inferred from automation.

The import increment adds ten axe audit states with zero reported violations:
file selection, column mapping, row review, receipt, and balance-check results
on desktop and 320/390px screens. It fixes secondary-text contrast on the new
summary backgrounds and moves focus to the next step heading. Browser scenarios
cover a semicolon/debit-credit/decimal-comma bank file, starting-date and amount
correction, invalid currency/date rows, duplicate opt-in, lost-response receipt
recovery, repeat imports with zero writes, bank-date display in Los Angeles,
JPY, and creating the first account inside the flow. Saved balance matches are
marked stale when new ledger activity appears; checks never adjust the ledger.

## Repeat the optional browser audit

[`scripts/ui_ux_check.cjs`](../scripts/ui_ux_check.cjs) creates a unique synthetic
user and test records in a running development API, checks the interactions,
saves screenshots and `accessibility.json`, and exits unsuccessfully on an
interaction failure or accessibility violation. Run it against a development or
test instance. For repeatable automated runs, use `CATEGORIZER=keyword`,
`API_RATE_LIMIT_RPS=100`, and `API_RATE_LIMIT_BURST=200` in that instance's local
configuration. Keep the normal application dependencies unchanged.

Install the optional free tools in a temporary directory (tested with
Playwright 1.57.0 and axe-core 4.10.3):

```sh
npm install --prefix /tmp/ledgerx-ui-qa --no-save playwright@1.57.0 axe-core@4.10.3
/tmp/ledgerx-ui-qa/node_modules/.bin/playwright install chromium

# From the LedgerX repository root, with the API and UI already running:
PLAYWRIGHT_MODULE_PATH=/tmp/ledgerx-ui-qa/node_modules/playwright \
AXE_SCRIPT_PATH=/tmp/ledgerx-ui-qa/node_modules/axe-core/axe.min.js \
node scripts/ui_ux_check.cjs
```

Defaults are UI `http://127.0.0.1:5173`, API `http://127.0.0.1:8080`, and output
`/tmp/ledgerx-ui-ux`. Override them with `LEDGERX_UI_URL`, `LEDGERX_API_URL`, and
`LEDGERX_QA_OUTPUT`. An existing Chromium executable can be selected using
`PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH`. Container environments that require it
can set `PLAYWRIGHT_NO_SANDBOX=1`; leave it unset for normal local runs.

Run `node scripts/imports_check.cjs` with the same configuration to repeat the
import/balance workflow. It also uses Python 3's standard CSV reader to verify
a report generated by the running worker. Its default output directory is
`/tmp/ledgerx-imports-check` (screenshots, a synthetic report, and `results.json`).

Run `node scripts/workspace_check.cjs` for the five correction/recovery scenarios.
It uses the same configuration and Python 3's standard CSV reader. The default
output is `/tmp/ledgerx-workspace-check`; results explicitly identify synthetic
scenarios and `customerParticipants: 0`. The original thirteen-state UI audit,
ten-state import audit, and ten-state recovery audit together cover 33 states.

## Categories, budgets and interactive reports

The expanded local release check now covers **55 accessibility states** with
zero reported violations, including 12 new financial-management states. New
coverage includes category/subcategory icons, archive/unarchive, reviewed
transaction/plan reassignment, protection for attached budgets, monthly limits,
near/over status, previous-month copying, report balances and income drilldown,
interactive month trends, CSV period handoff, custom periods and fetch failure /
retry / empty states. Screens are checked at desktop and 320px widths. Automated
checks do not replace direct customer usability sessions.

Reports are a detailed review surface; Overview retains the daily summary.
Navigation now uses Overview, Activity, Budgets and Reports. Plans remain
accessible from these workflows and secondary navigation. Budget text explains
overlaps and distinguishes recorded spending from future commitments.

The recurring-planning stage expands the local release check to **66 states**.
Its 11 new states cover empty schedules, creation, income, overdue plans, actual
amount/date confirmation, skip/pause/edit/end and timing-sensitive projected
shortfalls on desktop and 320px screens. The estimate is collapsed by default
to keep the daily list calm. Contrast was checked against the estimate panel's
background as well as white wallet cards. Automated checks remain separate
from customer usability observations.

## Account management

The current release gate covers **93 accessibility states** with zero reported
violations on an isolated stack with synthetic records. It includes the 16-state
navigation/exact-money check and 11 account-management states. The account check
covers rename, duplicate names, keyboard focus and Escape, stale edits with an
explicit reload, recurring-bill archive restrictions, retained reports/history,
archived transfer choices, all-archived Overview, status filters and restore.
Desktop and 320px layouts were inspected. Currency-label contrast and the status
filter's accessible label were corrected during verification. This remains
automated and developer verification, not customer usability validation.
