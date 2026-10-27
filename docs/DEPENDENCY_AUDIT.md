# Dependency audit — 2026-10-28

Repository: `EftikharAzim/LedgerX`. Baseline commit: `96afd05`.

## Audit scope and result

The authenticated GitHub Dependabot API returned **107 open alerts**: **9 critical, 40 high, 53 medium, and 5 low**. These cover **24 packages**, with **20 Go alerts and 87 npm alerts**. GitHub classified 68 alerts as runtime and 39 as development dependencies. This snapshot supersedes the earlier push notification's count of 100.

Every captured alert's vulnerable version range was compared with the resolved Go module graph or every matching package in the updated npm lockfile. **None of the 107 captured ranges matches the updated graph.** The per-alert evidence is below.

The baseline npm audit reported 23 affected packages (17 high, 5 moderate, 1 low). After the update, npm audit reported **0 vulnerabilities**, including development dependencies. npm's package totals and GitHub's individual alert totals measure different things.

The baseline govulncheck source scan identified 54 unique advisory IDs, of which 22 had static call paths. After the update, the source scan reported **0 vulnerabilities in called code and 0 in imported packages**. A module-only inventory still flags the unused OpenPGP package described below.

These are local remediation results. No alerts were dismissed, and no claim is made that GitHub has closed them. Dependabot must rescan the updated manifests after they are published to the default branch.

## Upgrade choices

- The application minimum, CI toolchain selection, and Docker builder move to **Go 1.26.10**. The prior module minimum and Docker/CI line were Go 1.25; the local baseline scan used Go 1.26.4. The Go scan found standard-library issues in paths involving HTTP, TLS, filesystem confinement, and certificate verification. The [official download index](https://go.dev/dl/?mode=json) lists Go 1.26.10 as the current patched release on that maintained line.
- chi moves to v5.4.0 and pgx to v5.9.2, clearing their captured advisory ranges. The Go `x/crypto`, `x/sys`, and `x/text` modules move to patched releases; `x/sync` follows the resulting module requirements.
- Frontend direct dependencies retain their existing major versions. Manifest minimums are raised for axios, React Router DOM, Zod, PostCSS, and Vite; compatible dependencies are refreshed in the lockfile. No `--force` audit fix or dependency override is used.
- The old vulnerable glob package is removed from the refreshed dependency graph. Old minimatch/brace-expansion branches are either patched or replaced through compatible parent dependency updates.
- `make audit` runs govulncheck v1.9.0 and npm audit. Both CI workflows run the corresponding audit. The Go gate uses symbol analysis, which [checks call paths](https://go.dev/doc/security/vuln/); it does not falsely label an unused module package as an application call.
- Go lint excludes the frontend's generated `node_modules` tree. The updated flatted npm package ships Go sources there; those upstream files are not service source and must not trigger application documentation rules.

## Exposure and residual finding

The nine critical GitHub alerts belong to `x/crypto` (seven SSH/agent advisories) and pgx (two advisories). The baseline Go scan did not find call paths for those nine individual advisories. That is a reachability observation, not a reason to keep vulnerable versions. All affected modules are upgraded.

The baseline scanner did find a call path for the later pgx placeholder-confusion advisory and for `x/text` advisories, along with standard-library advisories. A static call path does not prove that an attacker can satisfy every advisory's preconditions; no exploit was attempted.

Several JavaScript advisories concern SSR, Node HTTP adapters, or development tooling. LedgerX's UI is a browser SPA; this audit does not claim a JavaScript reachability proof. Runtime and development dependencies were both remediated.

[GO-2026-5932](https://pkg.go.dev/vuln/GO-2026-5932) concerns `golang.org/x/crypto/openpgp` and has no known fixed version. The module remains needed for bcrypt and database authentication dependencies, but OpenPGP is not imported by the application. The symbol scan and package inventory both confirm this distinction. It is retained as an explicit module-level residual, not dismissed or reported as patched.

The installed ESLint 9 line emits an upstream end-of-support deprecation notice. npm audit reports no known vulnerability for that selected version. A major ESLint migration is outside this security patch.

## Verification

| Check | Result |
| --- | --- |
| Comparison with all 107 captured Dependabot ranges | Passed: no vulnerable resolved version remains |
| npm audit, including development dependencies | Passed: 0 vulnerabilities |
| govulncheck source/symbol scan on Go 1.26.10 | Passed: 0 called-code or imported-package vulnerabilities |
| Module-only inventory from `cmd/ledgerx` | One unused, unfixable OpenPGP advisory; expected vulnerable-module exit status, not a passing module scan |
| Full Go tests with race detector and real PostgreSQL/Redis | Passed on Go 1.26.10; integration tests ran rather than skipped |
| Go lint, vet, build, module checksums, and sqlc consistency | Passed; lint reports 0 issues and all module checksums verify |
| Frontend tests, lint, production build, and Node 24 checks | Passed: 7 tests on host Node 26 and a clean Node 24 Linux container; clean install, lint, build, and audit also passed in that container |
| Smoke-target failure/cleanup checks | Passed: 3 regression tests |
| Live end-to-end HTTP smoke on the upgraded stack | Passed: `SMOKE OK`, using the rebuilt Go 1.26.10 API plus isolated PostgreSQL/Redis |

The smoke covers registration, refresh rotation, account creation, idempotent
transactions, transfers, balances, history, summaries, reversal, asynchronous
CSV export, authenticated download, and logout. Binary build metadata confirms
the patched chi, pgx, crypto, sys, and text module versions were used.

The frontend build emits non-fatal upstream Zod PURE-comment annotation notices
and a chunk-size notice for the 553.14 kB minified JavaScript bundle. Live Claude
and S3 were not exercised; the smoke uses the deterministic keyword categorizer
and local export storage. The Docker builder tag was checked in the registry;
a full application image build and image/OS vulnerability scan were not run.

Application logic and database schema are unchanged by this patch. The dependency manifests, lockfiles, toolchain, CI, and Docker builder should be treated as one upgrade. Reverting them restores the earlier versions and their vulnerability exposure; no ledger-data rollback is involved.

## Package remediation inventory

| Ecosystem | Package | Baseline resolved versions | Updated resolved versions | Captured alerts |
| --- | --- | --- | --- | --- |
| go | `github.com/go-chi/chi/v5` | `v5.2.3` | `v5.4.0` | 2 |
| go | `github.com/jackc/pgx/v5` | `v5.7.5` | `v5.9.2` | 3 |
| go | `golang.org/x/crypto` | `v0.42.0` | `v0.58.0` | 15 |
| npm | `@babel/core` | `7.28.4` | `7.29.7` | 1 |
| npm | `@humanfs/node` | `0.16.7` | `0.16.8` | 1 |
| npm | `axios` | `1.12.2` | `1.21.0` | 32 |
| npm | `baseline-browser-mapping` | `2.8.13` | `2.11.34` | 1 |
| npm | `brace-expansion` | `2.0.2`, `1.1.12` | `5.0.12`, `1.1.21` | 4 |
| npm | `browserslist` | `4.26.3` | `4.29.5` | 1 |
| npm | `flatted` | `3.3.3` | `3.4.4` | 1 |
| npm | `follow-redirects` | `1.15.11` | `1.16.2` | 1 |
| npm | `form-data` | `4.0.4` | `4.0.6` | 1 |
| npm | `glob` | `10.4.5` | Removed | 1 |
| npm | `js-yaml` | `4.1.0` | `4.3.2` | 5 |
| npm | `minimatch` | `9.0.5`, `3.1.2` | `10.2.6`, `3.1.5` | 2 |
| npm | `nanoid` | `3.3.11` | `3.3.20` | 1 |
| npm | `picomatch` | `2.3.1`, `4.0.3` | `2.3.2`, `4.0.7` | 2 |
| npm | `postcss` | `8.5.6` | `8.5.32` | 4 |
| npm | `postcss-selector-parser` | `6.1.2` | `6.1.5` | 2 |
| npm | `react-router` | `7.9.3` | `7.18.4` | 13 |
| npm | `rollup` | `4.52.4` | `4.65.1` | 1 |
| npm | `source-map-js` | `1.2.1` | `1.2.2` | 1 |
| npm | `vite` | `7.1.9` | `7.3.7` | 11 |
| npm | `zod` | `4.1.12` | `4.6.7` | 1 |

Additional Go security changes outside the captured Dependabot package list:

| Component | Before | After |
| --- | --- | --- |
| Local Go scan/build toolchain | 1.26.4 | 1.26.10 |
| `golang.org/x/sys` | v0.36.0 | v0.49.0 |
| `golang.org/x/text` | v0.29.0 | v0.43.0 |
| `golang.org/x/sync` | v0.17.0 | v0.24.0 |

## Captured alerts and range comparison

Ranges and first patched versions come from each GitHub alert. Every resolved occurrence was checked; newer replacement major versions are evaluated against the alert's complete range rather than compared only to its first patched version.

| Alert | Package | Severity | Advisory | Vulnerable range | First patched | Updated graph |
| --- | --- | --- | --- | --- | --- | --- |
| [#1](https://github.com/EftikharAzim/LedgerX/security/dependabot/1) | `vite` | medium | [GHSA-93m4-6634-74q7](https://github.com/advisories/GHSA-93m4-6634-74q7) | `>= 7.1.0, <= 7.1.10` | `7.1.11` | `7.3.7` |
| [#4](https://github.com/EftikharAzim/LedgerX/security/dependabot/4) | `js-yaml` | medium | [GHSA-mh29-5h37-fv8m](https://github.com/advisories/GHSA-mh29-5h37-fv8m) | `>= 4.0.0, < 4.1.1` | `4.1.1` | `4.3.2` |
| [#5](https://github.com/EftikharAzim/LedgerX/security/dependabot/5) | `golang.org/x/crypto` | medium | [GHSA-j5w8-q4qc-rx2x](https://github.com/advisories/GHSA-j5w8-q4qc-rx2x) | `< 0.45.0` | `0.45.0` | `v0.58.0` |
| [#6](https://github.com/EftikharAzim/LedgerX/security/dependabot/6) | `golang.org/x/crypto` | medium | [GHSA-f6x5-jh6r-wrfv](https://github.com/advisories/GHSA-f6x5-jh6r-wrfv) | `< 0.45.0` | `0.45.0` | `v0.58.0` |
| [#7](https://github.com/EftikharAzim/LedgerX/security/dependabot/7) | `glob` | high | [GHSA-5j98-mcp5-4vw2](https://github.com/advisories/GHSA-5j98-mcp5-4vw2) | `>= 10.2.0, < 10.5.0` | `10.5.0` | Removed |
| [#9](https://github.com/EftikharAzim/LedgerX/security/dependabot/9) | `react-router` | medium | [GHSA-9jcx-v3wj-wh4m](https://github.com/advisories/GHSA-9jcx-v3wj-wh4m) | `>= 7.0.0, < 7.9.6` | `7.9.6` | `7.18.4` |
| [#11](https://github.com/EftikharAzim/LedgerX/security/dependabot/11) | `react-router` | high | [GHSA-8v8x-cx79-35w7](https://github.com/advisories/GHSA-8v8x-cx79-35w7) | `>= 7.0.0, < 7.12.0` | `7.12.0` | `7.18.4` |
| [#13](https://github.com/EftikharAzim/LedgerX/security/dependabot/13) | `react-router` | high | [GHSA-2w69-qvjg-hvjx](https://github.com/advisories/GHSA-2w69-qvjg-hvjx) | `>= 7.0.0, <= 7.11.0` | `7.12.0` | `7.18.4` |
| [#15](https://github.com/EftikharAzim/LedgerX/security/dependabot/15) | `react-router` | medium | [GHSA-h5cw-625j-3rxh](https://github.com/advisories/GHSA-h5cw-625j-3rxh) | `>= 7.0.0, <= 7.11.0` | `7.12.0` | `7.18.4` |
| [#20](https://github.com/EftikharAzim/LedgerX/security/dependabot/20) | `axios` | high | [GHSA-43fc-jf86-j433](https://github.com/advisories/GHSA-43fc-jf86-j433) | `>= 1.0.0, <= 1.13.4` | `1.13.5` | `1.21.0` |
| [#25](https://github.com/EftikharAzim/LedgerX/security/dependabot/25) | `rollup` | high | [GHSA-mw96-cpmx-2vgc](https://github.com/advisories/GHSA-mw96-cpmx-2vgc) | `>= 4.0.0, < 4.59.0` | `4.59.0` | `4.65.1` |
| [#26](https://github.com/EftikharAzim/LedgerX/security/dependabot/26) | `minimatch` | high | [GHSA-7r86-cg39-jmmj](https://github.com/advisories/GHSA-7r86-cg39-jmmj) | `>= 9.0.0, < 9.0.7` | `9.0.7` | `10.2.6`, `3.1.5` |
| [#29](https://github.com/EftikharAzim/LedgerX/security/dependabot/29) | `minimatch` | high | [GHSA-7r86-cg39-jmmj](https://github.com/advisories/GHSA-7r86-cg39-jmmj) | `< 3.1.3` | `3.1.3` | `10.2.6`, `3.1.5` |
| [#31](https://github.com/EftikharAzim/LedgerX/security/dependabot/31) | `flatted` | high | [GHSA-rf6f-7fwh-wjgh](https://github.com/advisories/GHSA-rf6f-7fwh-wjgh) | `<= 3.4.1` | `3.4.2` | `3.4.4` |
| [#32](https://github.com/EftikharAzim/LedgerX/security/dependabot/32) | `picomatch` | medium | [GHSA-3v7f-55p6-f55p](https://github.com/advisories/GHSA-3v7f-55p6-f55p) | `>= 4.0.0, < 4.0.4` | `4.0.4` | `2.3.2`, `4.0.7` |
| [#35](https://github.com/EftikharAzim/LedgerX/security/dependabot/35) | `vite` | high | [GHSA-v2wj-q39q-566r](https://github.com/advisories/GHSA-v2wj-q39q-566r) | `>= 7.1.0, <= 7.3.1` | `7.3.2` | `7.3.7` |
| [#36](https://github.com/EftikharAzim/LedgerX/security/dependabot/36) | `vite` | high | [GHSA-p9ff-h696-f583](https://github.com/advisories/GHSA-p9ff-h696-f583) | `>= 7.0.0, <= 7.3.1` | `7.3.2` | `7.3.7` |
| [#37](https://github.com/EftikharAzim/LedgerX/security/dependabot/37) | `vite` | medium | [GHSA-4w7w-66w2-5vf9](https://github.com/advisories/GHSA-4w7w-66w2-5vf9) | `>= 7.0.0, <= 7.3.1` | `7.3.2` | `7.3.7` |
| [#39](https://github.com/EftikharAzim/LedgerX/security/dependabot/39) | `picomatch` | medium | [GHSA-3v7f-55p6-f55p](https://github.com/advisories/GHSA-3v7f-55p6-f55p) | `< 2.3.2` | `2.3.2` | `2.3.2`, `4.0.7` |
| [#47](https://github.com/EftikharAzim/LedgerX/security/dependabot/47) | `follow-redirects` | medium | [GHSA-r4q5-vmmm-2653](https://github.com/advisories/GHSA-r4q5-vmmm-2653) | `<= 1.15.11` | `1.16.0` | `1.16.2` |
| [#49](https://github.com/EftikharAzim/LedgerX/security/dependabot/49) | `axios` | medium | [GHSA-fvcv-3m26-pcqx](https://github.com/advisories/GHSA-fvcv-3m26-pcqx) | `>= 1.0.0, < 1.15.0` | `1.15.0` | `1.21.0` |
| [#50](https://github.com/EftikharAzim/LedgerX/security/dependabot/50) | `github.com/jackc/pgx/v5` | critical | [GHSA-9jj7-4m8r-rfcm](https://github.com/advisories/GHSA-9jj7-4m8r-rfcm) | `< 5.9.0` | `5.9.0` | `v5.9.2` |
| [#52](https://github.com/EftikharAzim/LedgerX/security/dependabot/52) | `axios` | medium | [GHSA-3p68-rc4w-qgx5](https://github.com/advisories/GHSA-3p68-rc4w-qgx5) | `>= 1.0.0, < 1.15.0` | `1.15.0` | `1.21.0` |
| [#53](https://github.com/EftikharAzim/LedgerX/security/dependabot/53) | `github.com/jackc/pgx/v5` | low | [GHSA-j88v-2chj-qfwx](https://github.com/advisories/GHSA-j88v-2chj-qfwx) | `< 5.9.2` | `5.9.2` | `v5.9.2` |
| [#55](https://github.com/EftikharAzim/LedgerX/security/dependabot/55) | `axios` | high | [GHSA-6chq-wfr3-2hj9](https://github.com/advisories/GHSA-6chq-wfr3-2hj9) | `>= 1.0.0, < 1.15.1` | `1.15.1` | `1.21.0` |
| [#57](https://github.com/EftikharAzim/LedgerX/security/dependabot/57) | `axios` | medium | [GHSA-vf2m-468p-8v99](https://github.com/advisories/GHSA-vf2m-468p-8v99) | `>= 1.0.0, < 1.15.1` | `1.15.1` | `1.21.0` |
| [#59](https://github.com/EftikharAzim/LedgerX/security/dependabot/59) | `axios` | medium | [GHSA-3w6x-2g7m-8v23](https://github.com/advisories/GHSA-3w6x-2g7m-8v23) | `>= 1.0.0, < 1.15.2` | `1.15.2` | `1.21.0` |
| [#61](https://github.com/EftikharAzim/LedgerX/security/dependabot/61) | `axios` | low | [GHSA-xhjh-pmcv-23jw](https://github.com/advisories/GHSA-xhjh-pmcv-23jw) | `>= 1.0.0, < 1.15.1` | `1.15.1` | `1.21.0` |
| [#63](https://github.com/EftikharAzim/LedgerX/security/dependabot/63) | `axios` | high | [GHSA-pf86-5x62-jrwf](https://github.com/advisories/GHSA-pf86-5x62-jrwf) | `>= 1.0.0, < 1.15.1` | `1.15.1` | `1.21.0` |
| [#65](https://github.com/EftikharAzim/LedgerX/security/dependabot/65) | `axios` | medium | [GHSA-m7pr-hjqh-92cm](https://github.com/advisories/GHSA-m7pr-hjqh-92cm) | `>= 1.0.0, < 1.15.1` | `1.15.1` | `1.21.0` |
| [#67](https://github.com/EftikharAzim/LedgerX/security/dependabot/67) | `axios` | medium | [GHSA-445q-vr5w-6q77](https://github.com/advisories/GHSA-445q-vr5w-6q77) | `>= 1.0.0, < 1.15.1` | `1.15.1` | `1.21.0` |
| [#69](https://github.com/EftikharAzim/LedgerX/security/dependabot/69) | `axios` | medium | [GHSA-5c9x-8gcm-mpgx](https://github.com/advisories/GHSA-5c9x-8gcm-mpgx) | `>= 1.0.0, < 1.15.1` | `1.15.1` | `1.21.0` |
| [#71](https://github.com/EftikharAzim/LedgerX/security/dependabot/71) | `axios` | medium | [GHSA-xx6v-rp6x-q39c](https://github.com/advisories/GHSA-xx6v-rp6x-q39c) | `>= 1.0.0, < 1.15.1` | `1.15.1` | `1.21.0` |
| [#73](https://github.com/EftikharAzim/LedgerX/security/dependabot/73) | `axios` | high | [GHSA-q8qp-cvcw-x6jj](https://github.com/advisories/GHSA-q8qp-cvcw-x6jj) | `>= 1.0.0, < 1.15.2` | `1.15.2` | `1.21.0` |
| [#75](https://github.com/EftikharAzim/LedgerX/security/dependabot/75) | `axios` | medium | [GHSA-62hf-57xw-28j9](https://github.com/advisories/GHSA-62hf-57xw-28j9) | `>= 1.0.0, < 1.15.1` | `1.15.1` | `1.21.0` |
| [#77](https://github.com/EftikharAzim/LedgerX/security/dependabot/77) | `axios` | high | [GHSA-pmwg-cvhr-8vh7](https://github.com/advisories/GHSA-pmwg-cvhr-8vh7) | `>= 1.0.0, < 1.15.1` | `1.15.1` | `1.21.0` |
| [#79](https://github.com/EftikharAzim/LedgerX/security/dependabot/79) | `react-router` | high | [GHSA-8646-j5j9-6r62](https://github.com/advisories/GHSA-8646-j5j9-6r62) | `>= 7.7.0, < 7.13.2` | `7.13.2` | `7.18.4` |
| [#81](https://github.com/EftikharAzim/LedgerX/security/dependabot/81) | `react-router` | medium | [GHSA-f22v-gfqf-p8f3](https://github.com/advisories/GHSA-f22v-gfqf-p8f3) | `>= 7.5.1, < 7.13.2` | `7.13.2` | `7.18.4` |
| [#83](https://github.com/EftikharAzim/LedgerX/security/dependabot/83) | `react-router` | high | [GHSA-49rj-9fvp-4h2h](https://github.com/advisories/GHSA-49rj-9fvp-4h2h) | `>= 7.0.0, <= 7.14.1` | `7.14.2` | `7.18.4` |
| [#85](https://github.com/EftikharAzim/LedgerX/security/dependabot/85) | `react-router` | medium | [GHSA-2j2x-hqr9-3h42](https://github.com/advisories/GHSA-2j2x-hqr9-3h42) | `>= 7.0.0, < 7.14.1` | `7.14.1` | `7.18.4` |
| [#87](https://github.com/EftikharAzim/LedgerX/security/dependabot/87) | `react-router` | high | [GHSA-8x6r-g9mw-2r78](https://github.com/advisories/GHSA-8x6r-g9mw-2r78) | `>= 7.0.0, < 7.15.0` | `7.15.0` | `7.18.4` |
| [#89](https://github.com/EftikharAzim/LedgerX/security/dependabot/89) | `react-router` | high | [GHSA-rxv8-25v2-qmq8](https://github.com/advisories/GHSA-rxv8-25v2-qmq8) | `>= 7.0.0, < 7.14.0` | `7.14.0` | `7.18.4` |
| [#91](https://github.com/EftikharAzim/LedgerX/security/dependabot/91) | `axios` | high | [GHSA-777c-7fjr-54vf](https://github.com/advisories/GHSA-777c-7fjr-54vf) | `>= 1.7.0, < 1.16.0` | `1.16.0` | `1.21.0` |
| [#93](https://github.com/EftikharAzim/LedgerX/security/dependabot/93) | `axios` | high | [GHSA-hfxv-24rg-xrqf](https://github.com/advisories/GHSA-hfxv-24rg-xrqf) | `>= 1.0.0, < 1.16.0` | `1.16.0` | `1.21.0` |
| [#95](https://github.com/EftikharAzim/LedgerX/security/dependabot/95) | `axios` | high | [GHSA-p92q-9vqr-4j8v](https://github.com/advisories/GHSA-p92q-9vqr-4j8v) | `>= 1.0.0, < 1.16.0` | `1.16.0` | `1.21.0` |
| [#97](https://github.com/EftikharAzim/LedgerX/security/dependabot/97) | `axios` | high | [GHSA-j5f8-grm9-p9fc](https://github.com/advisories/GHSA-j5f8-grm9-p9fc) | `>= 1.0.0, < 1.16.0` | `1.16.0` | `1.21.0` |
| [#99](https://github.com/EftikharAzim/LedgerX/security/dependabot/99) | `axios` | high | [GHSA-3g43-6gmg-66jw](https://github.com/advisories/GHSA-3g43-6gmg-66jw) | `>= 1.0.0, < 1.15.2` | `1.15.2` | `1.21.0` |
| [#103](https://github.com/EftikharAzim/LedgerX/security/dependabot/103) | `axios` | medium | [GHSA-w9j2-pvgh-6h63](https://github.com/advisories/GHSA-w9j2-pvgh-6h63) | `>= 1.0.0, < 1.15.1` | `1.15.1` | `1.21.0` |
| [#105](https://github.com/EftikharAzim/LedgerX/security/dependabot/105) | `axios` | high | [GHSA-35jp-ww65-95wh](https://github.com/advisories/GHSA-35jp-ww65-95wh) | `>= 1.0.0, < 1.16.0` | `1.16.0` | `1.21.0` |
| [#107](https://github.com/EftikharAzim/LedgerX/security/dependabot/107) | `postcss` | medium | [GHSA-qx2v-qp2m-jg93](https://github.com/advisories/GHSA-qx2v-qp2m-jg93) | `< 8.5.10` | `8.5.10` | `8.5.32` |
| [#108](https://github.com/EftikharAzim/LedgerX/security/dependabot/108) | `axios` | medium | [GHSA-898c-q2cr-xwhg](https://github.com/advisories/GHSA-898c-q2cr-xwhg) | `>= 1.0.0, < 1.16.0` | `1.16.0` | `1.21.0` |
| [#110](https://github.com/EftikharAzim/LedgerX/security/dependabot/110) | `vite` | medium | [GHSA-v6wh-96g9-6wx3](https://github.com/advisories/GHSA-v6wh-96g9-6wx3) | `>= 7.0.0, <= 7.3.4` | `7.3.5` | `7.3.7` |
| [#111](https://github.com/EftikharAzim/LedgerX/security/dependabot/111) | `vite` | medium | [GHSA-fx2h-pf6j-xcff](https://github.com/advisories/GHSA-fx2h-pf6j-xcff) | `>= 7.0.0, <= 7.3.4` | `7.3.5` | `7.3.7` |
| [#112](https://github.com/EftikharAzim/LedgerX/security/dependabot/112) | `github.com/go-chi/chi/v5` | medium | [GHSA-mqqf-5wvp-8fh8](https://github.com/advisories/GHSA-mqqf-5wvp-8fh8) | `>= 5.2.2, < 5.2.4` | `5.2.4` | `v5.4.0` |
| [#113](https://github.com/EftikharAzim/LedgerX/security/dependabot/113) | `form-data` | high | [GHSA-hmw2-7cc7-3qxx](https://github.com/advisories/GHSA-hmw2-7cc7-3qxx) | `>= 4.0.0, < 4.0.6` | `4.0.6` | `4.0.6` |
| [#115](https://github.com/EftikharAzim/LedgerX/security/dependabot/115) | `@babel/core` | low | [GHSA-4x5r-pxfx-6jf8](https://github.com/advisories/GHSA-4x5r-pxfx-6jf8) | `<= 7.29.0` | `7.29.6` | `7.29.7` |
| [#116](https://github.com/EftikharAzim/LedgerX/security/dependabot/116) | `github.com/jackc/pgx/v5` | critical | [GHSA-xgrm-4fwx-7qm8](https://github.com/advisories/GHSA-xgrm-4fwx-7qm8) | `< 5.9.0` | `5.9.0` | `v5.9.2` |
| [#117](https://github.com/EftikharAzim/LedgerX/security/dependabot/117) | `js-yaml` | medium | [GHSA-h67p-54hq-rp68](https://github.com/advisories/GHSA-h67p-54hq-rp68) | `>= 4.0.0, <= 4.1.1` | `4.2.0` | `4.3.2` |
| [#118](https://github.com/EftikharAzim/LedgerX/security/dependabot/118) | `golang.org/x/crypto` | critical | [GHSA-89gr-r52h-f8rx](https://github.com/advisories/GHSA-89gr-r52h-f8rx) | `< 0.52.0` | `0.52.0` | `v0.58.0` |
| [#119](https://github.com/EftikharAzim/LedgerX/security/dependabot/119) | `golang.org/x/crypto` | critical | [GHSA-rm3j-f69w-wqmq](https://github.com/advisories/GHSA-rm3j-f69w-wqmq) | `< 0.52.0` | `0.52.0` | `v0.58.0` |
| [#120](https://github.com/EftikharAzim/LedgerX/security/dependabot/120) | `golang.org/x/crypto` | high | [GHSA-q4h4-gmj2-qvw2](https://github.com/advisories/GHSA-q4h4-gmj2-qvw2) | `< 0.52.0` | `0.52.0` | `v0.58.0` |
| [#121](https://github.com/EftikharAzim/LedgerX/security/dependabot/121) | `golang.org/x/crypto` | medium | [GHSA-45gg-vh54-h5m9](https://github.com/advisories/GHSA-45gg-vh54-h5m9) | `< 0.52.0` | `0.52.0` | `v0.58.0` |
| [#122](https://github.com/EftikharAzim/LedgerX/security/dependabot/122) | `golang.org/x/crypto` | high | [GHSA-w879-237q-wc7r](https://github.com/advisories/GHSA-w879-237q-wc7r) | `< 0.52.0` | `0.52.0` | `v0.58.0` |
| [#123](https://github.com/EftikharAzim/LedgerX/security/dependabot/123) | `golang.org/x/crypto` | critical | [GHSA-5cgq-3rg8-m6cv](https://github.com/advisories/GHSA-5cgq-3rg8-m6cv) | `< 0.52.0` | `0.52.0` | `v0.58.0` |
| [#124](https://github.com/EftikharAzim/LedgerX/security/dependabot/124) | `golang.org/x/crypto` | critical | [GHSA-x527-x647-q7gg](https://github.com/advisories/GHSA-x527-x647-q7gg) | `< 0.52.0` | `0.52.0` | `v0.58.0` |
| [#125](https://github.com/EftikharAzim/LedgerX/security/dependabot/125) | `golang.org/x/crypto` | critical | [GHSA-jppx-rxg9-jmrx](https://github.com/advisories/GHSA-jppx-rxg9-jmrx) | `< 0.52.0` | `0.52.0` | `v0.58.0` |
| [#126](https://github.com/EftikharAzim/LedgerX/security/dependabot/126) | `golang.org/x/crypto` | critical | [GHSA-vgwf-h737-ff37](https://github.com/advisories/GHSA-vgwf-h737-ff37) | `< 0.52.0` | `0.52.0` | `v0.58.0` |
| [#127](https://github.com/EftikharAzim/LedgerX/security/dependabot/127) | `golang.org/x/crypto` | medium | [GHSA-78mq-xcr3-xm33](https://github.com/advisories/GHSA-78mq-xcr3-xm33) | `< 0.52.0` | `0.52.0` | `v0.58.0` |
| [#128](https://github.com/EftikharAzim/LedgerX/security/dependabot/128) | `golang.org/x/crypto` | critical | [GHSA-f5wc-c3c7-36mc](https://github.com/advisories/GHSA-f5wc-c3c7-36mc) | `< 0.52.0` | `0.52.0` | `v0.58.0` |
| [#129](https://github.com/EftikharAzim/LedgerX/security/dependabot/129) | `golang.org/x/crypto` | medium | [GHSA-9m57-25v3-79x9](https://github.com/advisories/GHSA-9m57-25v3-79x9) | `< 0.52.0` | `0.52.0` | `v0.58.0` |
| [#130](https://github.com/EftikharAzim/LedgerX/security/dependabot/130) | `golang.org/x/crypto` | medium | [GHSA-qpw4-5x99-6vjp](https://github.com/advisories/GHSA-qpw4-5x99-6vjp) | `< 0.52.0` | `0.52.0` | `v0.58.0` |
| [#131](https://github.com/EftikharAzim/LedgerX/security/dependabot/131) | `axios` | medium | [GHSA-jqh4-m9w3-8hp9](https://github.com/advisories/GHSA-jqh4-m9w3-8hp9) | `>= 1.7.0, < 1.18.0` | `1.18.0` | `1.21.0` |
| [#132](https://github.com/EftikharAzim/LedgerX/security/dependabot/132) | `axios` | medium | [GHSA-pmv8-rq9r-6j72](https://github.com/advisories/GHSA-pmv8-rq9r-6j72) | `>= 1.0.0, < 1.18.0` | `1.18.0` | `1.21.0` |
| [#133](https://github.com/EftikharAzim/LedgerX/security/dependabot/133) | `axios` | medium | [GHSA-42h9-826w-cgv3](https://github.com/advisories/GHSA-42h9-826w-cgv3) | `>= 1.0.0, < 1.18.0` | `1.18.0` | `1.21.0` |
| [#134](https://github.com/EftikharAzim/LedgerX/security/dependabot/134) | `axios` | medium | [GHSA-mmx7-hfxf-jppx](https://github.com/advisories/GHSA-mmx7-hfxf-jppx) | `>= 1.0.0, < 1.18.0` | `1.18.0` | `1.21.0` |
| [#135](https://github.com/EftikharAzim/LedgerX/security/dependabot/135) | `axios` | medium | [GHSA-7q8q-rj6j-mhjq](https://github.com/advisories/GHSA-7q8q-rj6j-mhjq) | `>= 1.0.0, < 1.18.0` | `1.18.0` | `1.21.0` |
| [#136](https://github.com/EftikharAzim/LedgerX/security/dependabot/136) | `js-yaml` | high | [GHSA-52cp-r559-cp3m](https://github.com/advisories/GHSA-52cp-r559-cp3m) | `>= 4.0.0, < 4.3.0` | `4.3.0` | `4.3.2` |
| [#137](https://github.com/EftikharAzim/LedgerX/security/dependabot/137) | `react-router` | high | [GHSA-chx6-hx7r-mcp5](https://github.com/advisories/GHSA-chx6-hx7r-mcp5) | `>= 7.0.0, < 7.18.0` | `7.18.0` | `7.18.4` |
| [#138](https://github.com/EftikharAzim/LedgerX/security/dependabot/138) | `react-router` | medium | [GHSA-337j-9hxr-rhxg](https://github.com/advisories/GHSA-337j-9hxr-rhxg) | `>= 6.4.0, < 7.18.0` | `7.18.0` | `7.18.4` |
| [#139](https://github.com/EftikharAzim/LedgerX/security/dependabot/139) | `react-router` | medium | [GHSA-wrjc-x8rr-h8h6](https://github.com/advisories/GHSA-wrjc-x8rr-h8h6) | `>= 6.0.0, < 7.18.0` | `7.18.0` | `7.18.4` |
| [#140](https://github.com/EftikharAzim/LedgerX/security/dependabot/140) | `brace-expansion` | high | [GHSA-3jxr-9vmj-r5cp](https://github.com/advisories/GHSA-3jxr-9vmj-r5cp) | `< 1.1.16` | `1.1.16` | `5.0.12`, `1.1.21` |
| [#141](https://github.com/EftikharAzim/LedgerX/security/dependabot/141) | `postcss` | high | [GHSA-6g55-p6wh-862q](https://github.com/advisories/GHSA-6g55-p6wh-862q) | `<= 8.5.11` | `8.5.12` | `8.5.32` |
| [#142](https://github.com/EftikharAzim/LedgerX/security/dependabot/142) | `postcss` | high | [GHSA-r28c-9q8g-f849](https://github.com/advisories/GHSA-r28c-9q8g-f849) | `<= 8.5.17` | `8.5.18` | `8.5.32` |
| [#145](https://github.com/EftikharAzim/LedgerX/security/dependabot/145) | `js-yaml` | high | [GHSA-5p4m-2wfm-xmqj](https://github.com/advisories/GHSA-5p4m-2wfm-xmqj) | `>= 4.0.0, < 4.3.1` | `4.3.1` | `4.3.2` |
| [#148](https://github.com/EftikharAzim/LedgerX/security/dependabot/148) | `postcss` | medium | [GHSA-fxqj-rqcc-2cmp](https://github.com/advisories/GHSA-fxqj-rqcc-2cmp) | `<= 8.5.22` | `8.5.23` | `8.5.32` |
| [#150](https://github.com/EftikharAzim/LedgerX/security/dependabot/150) | `brace-expansion` | high | [GHSA-3jxr-9vmj-r5cp](https://github.com/advisories/GHSA-3jxr-9vmj-r5cp) | `>= 2.0.0, < 2.1.2` | `2.1.2` | `5.0.12`, `1.1.21` |
| [#153](https://github.com/EftikharAzim/LedgerX/security/dependabot/153) | `postcss-selector-parser` | low | [GHSA-w9m9-85wc-3x92](https://github.com/advisories/GHSA-w9m9-85wc-3x92) | `>= 6.1.0, < 6.1.3` | `6.1.3` | `6.1.5` |
| [#154](https://github.com/EftikharAzim/LedgerX/security/dependabot/154) | `@humanfs/node` | medium | [GHSA-p498-v437-472g](https://github.com/advisories/GHSA-p498-v437-472g) | `< 0.16.8` | `0.16.8` | `0.16.8` |
| [#155](https://github.com/EftikharAzim/LedgerX/security/dependabot/155) | `nanoid` | high | [GHSA-xwg4-73v4-xw9w](https://github.com/advisories/GHSA-xwg4-73v4-xw9w) | `< 3.3.12` | `3.3.12` | `3.3.20` |
| [#157](https://github.com/EftikharAzim/LedgerX/security/dependabot/157) | `browserslist` | high | [GHSA-73wf-gq98-2v4g](https://github.com/advisories/GHSA-73wf-gq98-2v4g) | `<= 4.28.6` | `4.28.7` | `4.29.5` |
| [#158](https://github.com/EftikharAzim/LedgerX/security/dependabot/158) | `baseline-browser-mapping` | medium | [GHSA-w5vr-8v7q-w6rv](https://github.com/advisories/GHSA-w5vr-8v7q-w6rv) | `>= 2.0.0, < 2.11.0` | `2.11.0` | `2.11.34` |
| [#159](https://github.com/EftikharAzim/LedgerX/security/dependabot/159) | `js-yaml` | high | [GHSA-2883-xcg3-v3hh](https://github.com/advisories/GHSA-2883-xcg3-v3hh) | `>= 4.0.0, < 4.3.2` | `4.3.2` | `4.3.2` |
| [#160](https://github.com/EftikharAzim/LedgerX/security/dependabot/160) | `brace-expansion` | medium | [GHSA-q2hr-2g5m-vwhr](https://github.com/advisories/GHSA-q2hr-2g5m-vwhr) | `< 1.1.21` | `1.1.21` | `5.0.12`, `1.1.21` |
| [#161](https://github.com/EftikharAzim/LedgerX/security/dependabot/161) | `source-map-js` | high | [GHSA-68fv-2mgg-jv7q](https://github.com/advisories/GHSA-68fv-2mgg-jv7q) | `>= 1.0.0, < 1.2.2` | `1.2.2` | `1.2.2` |
| [#164](https://github.com/EftikharAzim/LedgerX/security/dependabot/164) | `vite` | medium | [GHSA-698v-4r3x-3vcv](https://github.com/advisories/GHSA-698v-4r3x-3vcv) | `>= 7.0.0, <= 7.3.6` | `7.3.7` | `7.3.7` |
| [#165](https://github.com/EftikharAzim/LedgerX/security/dependabot/165) | `vite` | high | [GHSA-587h-7wmj-h5pm](https://github.com/advisories/GHSA-587h-7wmj-h5pm) | `>= 7.0.0, <= 7.3.6` | `7.3.7` | `7.3.7` |
| [#166](https://github.com/EftikharAzim/LedgerX/security/dependabot/166) | `vite` | medium | [GHSA-r825-w8mm-5pxw](https://github.com/advisories/GHSA-r825-w8mm-5pxw) | `>= 7.0.0, <= 7.3.6` | `7.3.7` | `7.3.7` |
| [#167](https://github.com/EftikharAzim/LedgerX/security/dependabot/167) | `vite` | medium | [GHSA-rq7h-c2jc-7f22](https://github.com/advisories/GHSA-rq7h-c2jc-7f22) | `>= 7.0.0, <= 7.3.6` | `7.3.7` | `7.3.7` |
| [#168](https://github.com/EftikharAzim/LedgerX/security/dependabot/168) | `vite` | low | [GHSA-9jrq-w75r-8gcw](https://github.com/advisories/GHSA-9jrq-w75r-8gcw) | `>= 7.0.0, <= 7.3.6` | `7.3.7` | `7.3.7` |
| [#169](https://github.com/EftikharAzim/LedgerX/security/dependabot/169) | `github.com/go-chi/chi/v5` | medium | [GHSA-j6qf-3m9c-vwj3](https://github.com/advisories/GHSA-j6qf-3m9c-vwj3) | `<= 5.3.2` | `5.4.0` | `v5.4.0` |
| [#174](https://github.com/EftikharAzim/LedgerX/security/dependabot/174) | `brace-expansion` | medium | [GHSA-q2hr-2g5m-vwhr](https://github.com/advisories/GHSA-q2hr-2g5m-vwhr) | `>= 2.0.0, < 2.1.7` | `2.1.7` | `5.0.12`, `1.1.21` |
| [#175](https://github.com/EftikharAzim/LedgerX/security/dependabot/175) | `axios` | medium | [GHSA-9fr6-4gfg-395g](https://github.com/advisories/GHSA-9fr6-4gfg-395g) | `>= 1.0.0, < 1.20.0` | `1.20.0` | `1.21.0` |
| [#176](https://github.com/EftikharAzim/LedgerX/security/dependabot/176) | `axios` | medium | [GHSA-vh66-26gq-q6x8](https://github.com/advisories/GHSA-vh66-26gq-q6x8) | `>= 1.7.0, < 1.20.0` | `1.20.0` | `1.21.0` |
| [#177](https://github.com/EftikharAzim/LedgerX/security/dependabot/177) | `axios` | medium | [GHSA-4hqw-qxg8-jxx2](https://github.com/advisories/GHSA-4hqw-qxg8-jxx2) | `>= 1.12.0, < 1.20.0` | `1.20.0` | `1.21.0` |
| [#178](https://github.com/EftikharAzim/LedgerX/security/dependabot/178) | `axios` | medium | [GHSA-j8rh-479h-cp32](https://github.com/advisories/GHSA-j8rh-479h-cp32) | `>= 1.0.0, < 1.20.0` | `1.20.0` | `1.21.0` |
| [#179](https://github.com/EftikharAzim/LedgerX/security/dependabot/179) | `zod` | high | [GHSA-mrhx-c7xc-m3w5](https://github.com/advisories/GHSA-mrhx-c7xc-m3w5) | `< 4.6.6` | `4.6.6` | `4.6.7` |
| [#180](https://github.com/EftikharAzim/LedgerX/security/dependabot/180) | `postcss-selector-parser` | medium | [GHSA-rj75-hqrm-r3gf](https://github.com/advisories/GHSA-rj75-hqrm-r3gf) | `< 6.1.5` | `6.1.5` | `6.1.5` |
