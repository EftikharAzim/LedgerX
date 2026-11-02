# Financial accounts

Open **More → Accounts**, select an account, then choose **Edit account**.

- **Save name** changes its name wherever it appears. Currency cannot change;
  create a separate account to track a different currency.
- **Archive account** asks for review before removing the account from entry and
  transfer choices. Pause or end its active recurring bills first. You can follow
  **Manage recurring bills** from a blocked archive attempt.
- **Restore account** makes the account available for new activity again.
  Recurring bills stay paused or ended, and planned entries still require manual
  confirmation.

Archived accounts retain their balance, history, categories, reports and backup
records. Archiving does not transfer money or close a real bank account. Record
any actual closing transfer before archiving. Existing plans can be canceled;
restore the account before confirming that they happened.

Use **Account status** on Accounts to show all, active or archived accounts.
Overview balances include archived accounts in their original currency. Report
totals continue to include their posted activity.

If another tab changes an account while the editor is open, LedgerX asks you to
reload before applying a conflicting change. **Reload details** replaces the
unsaved name with current details. Retrying a completed update does not duplicate
financial activity.

The account-management browser check covers desktop and 320px phone layouts,
keyboard focus, duplicate names, stale edits, recurring-bill restrictions,
archive/restore, status filters and retained history/reports. Backend integration
tests also cover ownership, concurrent edits, original creation retries and a
backup/restore round trip. See [API.md](./API.md#accounts) for the update contract.
