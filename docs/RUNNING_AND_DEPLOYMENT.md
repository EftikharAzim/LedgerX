# Run, verify, and deploy LedgerX

The complete local package is separate from `deploy/docker-compose.yml`, which
remains developer infrastructure. It uses a separate Compose project/database
and never removes data volumes during normal start/stop/update commands.

## Complete local app

Requirements: a running Docker engine, Docker Compose v2, and OpenSSL. macOS
includes OpenSSL; Docker Desktop is one option for Docker/Compose. Go and Node
are only needed for development or host-side checks, not for running the package.

From the repository root on `codex/modern-ui`:

```sh
./scripts/app.sh start
```

Open `http://localhost:8088` on the same computer. This is a local browser
address, not a public customer link. The initial build downloads pinned
dependencies and base images; subsequent builds reuse Docker's cache.

The package builds and serves React from the Go process, runs migrations before
starting the app, and probes database/Redis readiness. It binds only the app's
port to loopback; PostgreSQL and Redis have no published host ports. Runtime
uses a non-root user, a read-only image filesystem, a temporary directory, and
a named export volume. The built UI uses the same origin as `/auth` and `/v1`;
no Vite development server or frontend environment file is needed.

On first startup, the script creates an ignored, mode-600 `.env.app` containing
random database/signing/metrics secrets. It preserves an existing file. This
package forces the free keyword categorizer and does not pass paid AI keys.

Useful commands:

```sh
./scripts/app.sh status
./scripts/app.sh logs
./scripts/app.sh stop
./scripts/app.sh start
```

`stop` preserves all named volumes. Avoid `docker compose down -v`, volume
pruning, or deleting `.env.app` when keeping your data. Docker does not change
an existing PostgreSQL role's password when you edit its environment setting.

For updates, take a backup, pull the branch with `git pull --ff-only`, then run
`start` again. A failed build leaves the old app running. After a successful
build, the script stops app writers before migration; a migration failure
leaves the app stopped for investigation rather than running through an
incomplete upgrade. Do not force a dirty migration version without diagnosis.

The packaged database starts separately from any earlier developer workspace.
Use the existing financial JSON backup/restore to move your records. Copying a
browser's localStorage does not move server records.

## Settings and isolated stacks

Set `LEDGERX_PORT` before the first start to choose another loopback port.
The same setting can be kept in `.env.app`. Advanced options are
`DB_MAX_CONNS` (default 10), `DB_MIN_CONNS` (default 2), and
`WORKER_CONCURRENCY` (package default 4; host binary default 10).
The pool is shared with workers. More connections are not automatically faster;
budget the total across instances against PostgreSQL's connection limit.

`LEDGERX_APP_ENV` selects a separate configuration file; `COMPOSE_PROJECT_NAME`
selects separate containers and volumes. Use both for an independent test or
restore, along with a different port. Do not point a new configuration at an
existing database volume with different credentials.

## Server data backup and restore

```sh
./scripts/app.sh backup
```

The script briefly stops the app and Redis, keeping PostgreSQL available for a
consistent dump. It writes `database.dump`, `exports.tar.gz`, `redis.tar.gz`,
`server.env`, and a `COMPLETE` marker under a new private directory in
`tmp/server-backups/`. It resumes services that were previously running, even
when backup fails. A missing `COMPLETE` marker means the backup is unfinished.

These files contain financial records, authentication data, and secrets. Keep
them private, ideally on an encrypted disk, and copy them off the server if
protection from disk loss is required. The current package does not schedule or
upload backups. This is an application-state backup, not an operating-system,
DNS, hosting, or TLS-certificate snapshot. Optional ingress certificates can be
reissued; retain your hostname and deployment settings separately.

Restore only a trusted backup into a **fresh, empty Compose project**:

```sh
COMPOSE_PROJECT_NAME=ledgerx-recovery \
LEDGERX_APP_ENV=.env.recovery \
LEDGERX_PORT=8089 \
./scripts/app.sh restore tmp/server-backups/REPLACE_WITH_BACKUP_DIRECTORY
```

The script refuses a destination containing any public-schema tables. It
restores PostgreSQL, report files, and Redis, applies pending migrations, and
starts the app. It creates fresh local secrets and removes restored refresh
tokens; users log in again with their passwords. Verify records and existing
report downloads before choosing to move traffic to the recovered instance.
If restore fails partway, investigate and use another fresh destination; the
script does not erase an occupied or partially restored database for a retry.

A server backup also snapshots password/recovery state at backup time. Handle
subsequent credential changes before reopening an older recovered service.
The backup may contain accounts deleted after its creation. Operators must
handle later deletion requests before making an older restored backup available
again. Retention and off-server recovery remain deployment responsibilities.
User-facing financial JSON backups exclude authentication and generated CSVs;
they serve a different purpose from this server backup.

## Release checks

On an isolated stack containing synthetic records only:

```sh
make browser-setup
LEDGERX_UI_URL=http://127.0.0.1:8088 make release-check
python3 scripts/package_check.py
```

Browser setup requires Node 24 and installs the locked Playwright/axe tools.
The API/release checks also require Python 3 and curl. The packaged app itself
does not need these host tools. `package_check.py` creates two fresh test
projects on ports 8095/8096, verifies app replacement and full server recovery,
refuses nonempty restore targets, and removes only those test projects/volumes.
It does not use the developer or normal app database.

The release runner creates synthetic users and records in the selected app;
never point it at a customer deployment. `LEDGERX_API_URL` can override the API
address; by default the runner uses the UI origin. Reports live in
`tmp/release-check/`. Existing screenshot/audit scripts retain their standalone
environment overrides. Use a separate output directory per run.

CI runs Go/UI checks and the packaged browser/recovery job on `main`, PRs to
`main`, and direct pushes to `codex/modern-ui`. Browser CI uses a higher per-user
request limit in its isolated rapid-regression stack; normal startup and load
tests retain 5 requests/sec. A local pass does not establish a remote CI result.

## Public HTTPS configuration

No public host or provider account has been configured. The owner confirmed
having no hosting account. Running the commands above does not publish a site.

For an existing Internet-reachable host with an owned or provider-issued DNS
name, `deploy/compose.public.yml` adds Caddy HTTPS in front of the app. It needs
DNS pointing at the host and inbound ports 80/443. The template keeps database
and Redis private, excludes metrics/admin paths from public routing, and
replaces untrusted forwarding headers. API trust is restricted to the dedicated
ingress network. Choose an unused `LEDGERX_NETWORK_SUBNET` if the default
`172.30.60.0/24` conflicts with another network.

Review and run the template only on the intended host, using a fresh project:

```sh
COMPOSE_PROJECT_NAME=ledgerx-demo \
LEDGERX_APP_ENV=.env.demo \
LEDGERX_DOMAIN=REPLACE_WITH_REAL_HOSTNAME \
LEDGERX_COMPOSE_OVERRIDE=deploy/compose.public.yml \
./scripts/app.sh start
```

The hostname above is a placeholder, not a working address. Config validation
does not establish DNS, certificate issuance, or access from another device.
A managed host may provide its own HTTPS ingress instead; configure its actual
proxy CIDRs, origins, persistent storage, and dependency connections rather
than trusting arbitrary forwarding headers or exposing development services.

Keep the zero-spend constraint. A custom domain and paid AI are unnecessary.
No provider's current free plan is certified for the complete persistent stack.
Attempts to read official [Render](https://render.com/docs/free) and
[Cloudflare Tunnel](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/do-more-with-tunnels/trycloudflare/)
documentation from this environment returned HTTP 403, so their current quotas
and terms remain unverified. Do not infer that either service meets the hosting
requirements. A temporary laptop tunnel, if selected later, depends on that
laptop staying online and is not an always-on hosting commitment.

## Managed cloud builds

The Dockerfile supports an optional BuildKit secret named `proxy_ca` in each
networked stage. Normal local builds require no secret. A managed environment
with an HTTPS inspection proxy must supply its CA through a temporary Compose
build override and retain normal TLS verification. Use the runtime skill's
Docker guidance; never copy proxy credentials or session CA files into images.

### Repeated local builds and disk space

If unused Docker build cache fills the local environment, `docker builder prune`
can reclaim it. Rebuilding dependencies afterward takes longer. The managed
environment's VFS storage used substantially more physical space than Docker's
logical cache-size display during this release's repeated builds; reclaiming
unused build cache resolved the build-space failure.
