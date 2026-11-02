#!/usr/bin/env bash
# Run against an isolated, already-started app. Creates synthetic workspaces.
set -euo pipefail
repo_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_dir"
: "${LEDGERX_UI_URL:?Set LEDGERX_UI_URL to the isolated app address}"
export LEDGERX_API_URL="${LEDGERX_API_URL:-$LEDGERX_UI_URL}"
export LEDGERX_BASE_URL="$LEDGERX_API_URL"
export PLAYWRIGHT_MODULE_PATH="${PLAYWRIGHT_MODULE_PATH:-$repo_dir/scripts/browser/node_modules/playwright}"
export AXE_SCRIPT_PATH="${AXE_SCRIPT_PATH:-$repo_dir/scripts/browser/node_modules/axe-core/axe.min.js}"
output_dir="${LEDGERX_RELEASE_OUTPUT:-$repo_dir/tmp/release-check}"
mkdir -p "$output_dir"
chmod 700 "$output_dir"
curl --fail --silent --show-error "$LEDGERX_API_URL/readyz" >/dev/null
python3 scripts/test_smoke_target.py
python3 scripts/test_customer_summary.py
# The legacy smoke log includes synthetic credentials; keep it private.
(umask 077; python3 scripts/smoke_test_stdlib.py > "$output_dir/api-smoke.log" 2>&1) || {
  echo "API smoke failed. Inspect the private log at $output_dir/api-smoke.log." >&2
  exit 1
}
for check in ui_ux imports money_management recurring workspace security product_ui account_management; do
  LEDGERX_QA_OUTPUT="$output_dir/$check" node "scripts/${check}_check.cjs"
done
echo "Release flows passed. Results: $output_dir"
