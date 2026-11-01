# Five-person customer test kit

This kit is ready for five potential personal-spending customers. No customer
sessions have been conducted by the agent. Automated browser scenarios use
synthetic records and do not establish customer demand or usability success.

Use a local app, a screen share, dummy records, and the observation sheet. No
paid research service, AI API, bank connection, or hosting purchase is needed.
See [WORKSPACE_RECOVERY.md](./WORKSPACE_RECOVERY.md) for editing and recovery.

## Recruit and prepare

Choose five people who already try to track personal spending: someone using
notes, a spreadsheet user, a manual finance-app user, someone using cash and bank
accounts, and someone managing refunds or travel spending. Those profiles are
sampling prompts, not claims that five people represent the whole market.

Allow 25–35 minutes per person. Include phone-sized use and at least one keyboard
session. Use anonymous IDs P1–P5; use a fresh demo login per participant and keep
real account details out of screenshots and notes. Keep the participant's
credentials with them. Obtain permission before recording or quoting them.

Start with these questions about actual recent behavior:

- How did you record your last few purchases? Show the method if comfortable.
- When did you last stop tracking, and what made it difficult?
- Tell me about the last time your records did not match your balance.
- What would you need to bring with you to switch from your current method?

Say: “We are checking the app. If something is confusing, that helps us improve
it. Please say what you expect as you work. You can stop at any point.”

Avoid demonstrating the task first. Let the person work, note hesitation, and
ask “What did you expect?” before helping. Record any assistance. Do not ask for
positive feedback or interpret a feature request as proof of demand.

## Tasks and expected records

Use USD for Bank/Cash, and choose **All time** when reviewing the supplied 2024
records. Set Bank's starting balance to 100.00 before January 2024 activity.
Cash starts at zero. The observer can prepare the dummy login; account creation
remains part of T1. The example CSV is
[`customer-tests/spending-sample.csv`](./customer-tests/spending-sample.csv).

| Task | Give the participant this goal | Observer checks |
| --- | --- | --- |
| T1 | Create Bank with 100 starting money and Cash with zero. Record a 5.20 coffee purchase under Food. | Find account setup and purchase entry unaided; starting money does not become income. |
| T2 | The coffee cost 5.90 and was paid from Cash. Find it, correct amount/account/note, and review the result. | One reviewed correction; original remains linked; Bank returns to 100 and Cash becomes -5.90. Note whether the history language is understandable. |
| T3 | Bring the sample bank history into Bank. Include its three real transactions and leave the repeated and invalid rows out. | Destination, explicit date/number interpretation, duplicate/invalid feedback, and receipt. Bank becomes 340.00; Cash stays -5.90. Importing again proposes no new ready rows. |
| T4 | Review all-time spending and move 20 from Bank to Cash. Explain what changed. | Income 250.00; spending 15.90, including the partial refund; transfer does not change either total. Bank becomes 320.00; Cash becomes 14.10. Ask what useful decision the view supports. |
| T5 | Your bank says 319.00 posted. Compare it with the records, then compare 320.00. | Difference -1.00, then a match; neither check changes the ledger. Ask how they would investigate a mismatch. |
| T6 | Save your workspace and recover it in a new empty demo workspace. | Download a JSON backup; review balances; use the explicit sign-out/new-workspace flow; restore once; accounts/categories/history/receipts/checks and appearance remain available. Generate a report if useful. |

T2's note/date can be left to the participant; the financial expectations depend
on the amount/account change, not specific prose or a particular local time.
If a task was skipped, record that explicitly. Do not mark a completed task
unaided when the observer gave directions.

## Record, analyze, and decide

Fill [`customer-tests/observations.csv`](./customer-tests/observations.csv).
For each task, use `unaided`, `assisted`, `failed`, or `skipped`. Leave unobserved
rows blank. Record duration only when measured; include a specific error,
distraction, quote, or workaround when relevant. Do not infer success from a
checkbox or a participant saying they like the app.

Generate an honest summary using Python 3's standard library:

```sh
python3 scripts/summarize_customer_tests.py \
  docs/customer-tests/observations.csv \
  --output docs/customer-tests/STATUS.md
```

The report separates blank/skipped tasks from attempts, refuses duplicate
participant/task rows, and highlights tasks with failures or assistance. Empty
templates produce zero observed participants and no customer findings.

Use at least four unaided completions out of five for each core task as an
initial product target, not an industry benchmark. When people struggle, change
the observed step and repeat that task. Follow up after two weeks to learn
whether they returned to review spending and can name a useful decision. Compare
with their previous method before claiming an advantage or prioritizing a new
feature. Record recurring obstacles, frequency, impact, and the smallest change
that would address them.

## Automated preparation

[`scripts/workspace_check.cjs`](../scripts/workspace_check.cjs) exercises five
synthetic workflows: everyday corrections, cash transfers, refunds/travel,
imported bank dates, and backup/recovery. It checks desktop/phone layouts,
accessibility states, lost responses, stale previews, and a real worker report.
It records `customerParticipants: 0` explicitly. Run it using the optional free
browser tooling described in [UI_UX_REVIEW.md](./UI_UX_REVIEW.md).
