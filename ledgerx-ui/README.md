# LedgerX web app

Responsive personal finance UI built with React, TypeScript, Vite and TanStack
Query. The Go API owns balances, reports, financial validation and persistence.

## Run locally

For the complete app, start Docker Desktop and run `./scripts/app.sh start` from
the repository root, then open `http://localhost:8088`. See the
[project README](../README.md) for setup, persistence and updates.

For frontend development, start the backend with `make dev` from the repository
root. In a second terminal, use Node 24 and run:

```sh
cd ledgerx-ui
npm ci
npm run dev
```

Open `http://localhost:5173`. By default, Vite proxies API requests to port 8080.
Set `VITE_API_BASE_URL` only for a separate API origin and configure the backend's
allowed origins accordingly. Production builds use the same origin.

## Code organization

- `src/app`: routes, query client and authentication session lifecycle.
- `src/api`: API calls, token rotation and cancellation across sessions.
- `src/pages`: customer workflows and server-state queries.
- `src/components`: dialogs and reusable interface elements.
- `src/lib`: exact money/date logic, category rules, retry identities, shared
  query invalidation and browser downloads.
- `tests`: Node tests for pure logic and authentication/workflow behavior.

Parse decimal inputs with the shared money helpers. Keep currencies separate
and let the API calculate recorded balances. Reuse `nextWriteAttempt` for retry
keys, `refreshLedger` for affected views, `downloadBlob` for exports, and existing
dialog/focus components. Plans affect recorded money only after confirmation.
Session changes cancel requests and clear private query data.

## Validate changes

```sh
npm test
npm run lint
npm run build
```

The [release gate](../docs/RUNNING_AND_DEPLOYMENT.md#release-checks) additionally
checks real browser workflows, mobile reflow, accessibility and recovery.
See the [engineering review](../docs/ENGINEERING_REVIEW.md) for architecture,
conventions, verification limits and remaining work.
