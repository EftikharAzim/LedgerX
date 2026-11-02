# LedgerX engineering and product standards

The owner prefers a restrained, practical product with plain language. Apply
these standards to future work, including changes generated with AI assistance.

## Product and interface

- Name screens and actions after their task: Budgets, Reports, Export CSV,
  New account. Avoid slogans, motivational copy and repeated reassurance.
- Use decoration only when it communicates something useful. Do not add mock
  documents, sparkles, static workspace avatars, vanity metrics or filler cards.
- Keep balances and the primary action easy to find. Show optional controls and
  explanations when relevant; retain financial caveats and destructive-action
  confirmations where customers need them.
- Keep every destination reachable on phones. Overview, Activity, Budgets and
  Reports are the core navigation; More groups management and secondary tools.
- Monetary amounts must retain their full precision and remain legible at
  narrow widths. Do not split their digits/decimals, truncate, or silently round
  them to fit. Test realistic BDT values as well as USD, JPY and supported limits.
- Keep terminology consistent. Distinguish financial accounts from sign-in and
  security settings; do not expose backend details without a customer need.
- Use real data for charts and clear empty states. Make no unsupported customer,
  security, capacity or product claims. Consult external sources when external
  facts are needed, and distinguish research from customer validation.
- Review page metadata, dialogs, empty states, success messages and secondary
  screens as well as main pages. Say what happened and what the user can do.
  Mark category suggestions explicitly; do not imply confirmation or AI accuracy.
  Keep optional account appearance controls collapsed and remove obsolete
  illustration styles when their markup is removed.

## Implementation and verification

- Preserve exact money, separate currencies, append-only ledger history,
  idempotent writes, backup compatibility and explicit confirmation of plans.
- Keep the app usable without paid services. Do not add paid APIs as a default.
- Prefer focused changes and existing components over structural rewrites.
- For UI changes, run frontend tests, lint and production build. Verify changed
  workflows in a real browser at phone and desktop widths, including keyboard
  access, readable amounts, reflow and error/empty states. Use the existing
  release gate when changes span navigation or multiple financial workflows.
- Use synthetic data and isolated test services; preserve application volumes.
  Do not commit private records, credentials, tool caches or generated evidence.

See `docs/ENGINEERING_REVIEW.md` and `docs/UI_UX_REVIEW.md` for architecture and
previous findings. Use `docs/RUNNING_AND_DEPLOYMENT.md` for local/release setup.
