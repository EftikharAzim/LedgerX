import { downloadBlob } from "../lib/download";
import { useEffect, useRef, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { formatMoney } from "../api/endpoints";
import {
  getWorkspaceState,
  downloadWorkspace,
  previewRestore,
  getRestore,
  commitRestore,
  type RestoreView,
  type RestoreReceipt,
  type WorkspaceSummary,
} from "../api/workspace";
import {
  readAppearance,
  saveAppearance,
  type AccountAppearance,
} from "../lib/account-appearance";
import { refreshLedger } from "../lib/refresh-ledger";
import { apiError } from "../lib/errors";
import Icon from "../components/Icon";
import SuccessNotice from "../components/SuccessNotice";

const MAX_BACKUP_BYTES = 32 * 1024 * 1024;

export default function Workspace() {
  const qc = useQueryClient();
  const [params, setParams] = useSearchParams();
  const id = Number(params.get("restore_id")) || 0;
  const state = useQuery({
    queryKey: ["workspace"],
    queryFn: getWorkspaceState,
  });
  const review = useQuery({
    queryKey: ["restore", id],
    queryFn: () => getRestore(id),
    enabled: id > 0,
    retry: false,
  });
  const [file, setFile] = useState<File | null>(null);
  const [notice, setNotice] = useState("");
  const download = useMutation({
    mutationFn: () => {
      const appearance: Record<number, AccountAppearance> = {};
      for (const a of state.data?.accounts ?? [])
        appearance[a.id] = readAppearance(a.id);
      return downloadWorkspace(appearance);
    },
    onSuccess: (blob) => {
      downloadBlob(
        blob,
        `ledgerx-workspace-${new Date().toISOString().slice(0, 10)}.ledgerx.json`,
      );
      setNotice(
        "Backup is ready. Check your browser's downloads and keep the file private.",
      );
    },
  });
  const inspect = useMutation({
    mutationFn: async () => {
      if (!file) throw new Error("Choose a workspace backup.");
      if (file.size > MAX_BACKUP_BYTES)
        throw new Error("Choose a backup no larger than 32 MiB.");
      let raw: string;
      try {
        raw = new TextDecoder("utf-8", { fatal: true }).decode(
          await file.arrayBuffer(),
        );
      } catch {
        throw new Error("Choose the original UTF-8 LedgerX workspace backup.");
      }
      return previewRestore(file.name, raw);
    },
    onSuccess: (v) => {
      qc.setQueryData(["restore", v.id], v);
      setParams({ restore_id: String(v.id) });
      void qc.invalidateQueries({ queryKey: ["workspace"] });
    },
  });
  const fresh = () => {
    setParams({});
    setFile(null);
    inspect.reset();
  };
  return (
    <div className="workspace-tools-page">
      <div className="page-heading">
        <div>
          <h1>Backup & restore</h1>
          <p className="muted">
            Keep a portable copy of your complete financial workspace.
          </p>
        </div>
        <Link className="text-link" to="/exports">
          Back to reports
          <Icon name="arrow" width="16" height="16" />
        </Link>
      </div>
      <SuccessNotice message={notice} onDismiss={() => setNotice("")} />
      {state.isError && (
        <p className="form-error" role="alert">
          Couldn't load workspace status.{" "}
          <button className="text-button" onClick={() => state.refetch()}>
            Try again
          </button>
        </p>
      )}
      {!id ? (
        <div className="workspace-tools-grid">
          <section className="import-panel" aria-labelledby="backup-title">
            <span className="workspace-tool-symbol">
              <Icon name="download" />
            </span>
            <h2 id="backup-title">Download a backup</h2>
            <p className="muted">
              Download accounts, categories, transactions, transfers, correction
              history, import receipts, and balance checks in one workspace
              backup.
            </p>
            <p className="input-hint">
              Your account icons and colors from this device are included. The
              file contains private financial data. Store it somewhere you
              trust.
            </p>
            <button
              className="btn btn-primary"
              disabled={!state.isSuccess || download.isPending}
              onClick={() => {
                setNotice("");
                download.mutate();
              }}
            >
              {download.isPending
                ? "Preparing backup…"
                : "Download workspace backup"}
              <Icon name="download" width="17" height="17" />
            </button>
            {download.isError && (
              <p className="form-error" role="alert">
                {apiError(download.error)}
              </p>
            )}
            <details className="workspace-tool-help">
              <summary>What this file includes</summary>
              <p>
                All booked financial records, including reversed entries and
                their links, starting balances, imported bank references, saved
                comparisons, and optional account appearance.
              </p>
              <p>
                Login credentials, unsaved reviews, generated report files,
                server settings, and background jobs are excluded. Reports can
                be generated again from the restored records. CSV reports use a
                different format.
              </p>
            </details>
          </section>
          <section className="import-panel" aria-labelledby="restore-title">
            <span className="workspace-tool-symbol">
              <Icon name="reverse" />
            </span>
            <h2 id="restore-title">Restore a backup</h2>
            <p className="muted">
              Choose a LedgerX workspace JSON backup. Review its accounts and
              balances before restoring.
            </p>
            <form
              className="account-form"
              onSubmit={(e) => {
                e.preventDefault();
                if (file) inspect.mutate();
              }}
            >
              <div>
                <label className="label" htmlFor="restore-file">
                  Workspace backup file
                </label>
                <input
                  id="restore-file"
                  className="input"
                  type="file"
                  accept=".json,application/json"
                  onChange={(e) => {
                    setFile(e.target.files?.[0] ?? null);
                    inspect.reset();
                  }}
                  disabled={inspect.isPending}
                  required
                />
                <p className="input-hint">
                  Up to 32 MiB. File integrity, balanced transactions,
                  correction links, and saved balance history are checked before
                  saving.
                </p>
              </div>
              {inspect.isError && (
                <p className="form-error" role="alert">
                  {apiError(inspect.error)}
                </p>
              )}
              <button className="btn" disabled={!file || inspect.isPending}>
                {inspect.isPending ? "Checking backup…" : "Preview restore"}
                <Icon name="arrow" width="17" height="17" />
              </button>
            </form>
            <p className="input-hint">
              Restore needs an empty workspace. You can review a file here while
              your current records stay available.
            </p>
          </section>
        </div>
      ) : review.isPending ? (
        <p role="status">Loading saved restore review…</p>
      ) : review.isError ? (
        <section className="import-panel">
          <p className="form-error" role="alert">
            {apiError(review.error)}
          </p>
          <div className="import-actions">
            <button className="btn" onClick={() => review.refetch()}>
              Try loading again
            </button>
            <button className="text-button" onClick={fresh}>
              Choose another backup
            </button>
          </div>
        </section>
      ) : review.data?.receipt ? (
        <RestoreComplete receipt={review.data.receipt} onNew={fresh} />
      ) : review.data ? (
        <RestoreReview
          key={id}
          view={review.data}
          empty={state.isSuccess && state.data.empty}
          statusKnown={state.isSuccess}
          onBack={fresh}
          onComplete={(receipt) => {
            qc.setQueryData(["restore", id], {
              ...review.data,
              status: "completed",
              receipt,
            });
            void qc.invalidateQueries({ queryKey: ["workspace"] });
          }}
        />
      ) : null}
      <details className="import-history">
        <summary>Recent restore reviews and receipts</summary>
        <p className="input-hint">
          Reviews expire after 24 hours. Completed receipts can be reopened
          safely.
        </p>
        {state.isPending ? (
          <p role="status">Loading…</p>
        ) : !state.data?.restores.length ? (
          <p className="muted">No restore reviews yet.</p>
        ) : (
          <ul>
            {state.data.restores.map((r) => (
              <li key={r.id}>
                <Link to={`/workspace?restore_id=${r.id}`}>
                  <strong>{r.filename}</strong>
                  <span>
                    {r.status === "completed" ? "Restored" : "Saved review"} ·{" "}
                    {new Date(r.created_at).toLocaleDateString()}
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        )}
      </details>
    </div>
  );
}

function BackupSummary({ summary }: { summary: WorkspaceSummary }) {
  return (
    <>
      <p className="input-hint">
        Backup created {new Date(summary.exported_at).toLocaleString()}.
      </p>
      <div className="import-counts">
        <span>
          <strong>{summary.accounts.length}</strong>{" "}
          {summary.accounts.length === 1 ? "account" : "accounts"}
        </span>
        <span>
          <strong>{summary.transaction_count}</strong>{" "}
          {summary.transaction_count === 1 ? "ledger record" : "ledger records"}
        </span>
        <span>
          <strong>{summary.category_count}</strong>{" "}
          {summary.category_count === 1 ? "category" : "categories"}
        </span>
        {!!summary.planned_count && (
          <span>
            <strong>{summary.planned_count}</strong> planned entries (including
            resolved plans)
          </span>
        )}
      </div>
      {!!summary.budget_count && (
        <p>{summary.budget_count} monthly budgets included.</p>
      )}
      {!!summary.schedule_count && (
        <p>{summary.schedule_count} recurring schedules included.</p>
      )}
      <ul className="restore-account-list">
        {summary.accounts.map((a) => (
          <li key={a.source_id}>
            <div>
              <strong>{a.name}</strong>
              <span>
                {a.currency}
                {!a.active ? " · archived" : ""}
              </span>
            </div>
            <strong>{formatMoney(a.balance_minor, a.currency)}</strong>
          </li>
        ))}
      </ul>
      <p className="input-hint">
        Includes {summary.correction_count}{" "}
        {summary.correction_count === 1
          ? "correction link"
          : "correction links"}
        , {summary.import_count}{" "}
        {summary.import_count === 1 ? "import receipt" : "import receipts"}, and{" "}
        {summary.check_count}{" "}
        {summary.check_count === 1 ? "balance check" : "balance checks"}.
        Currencies remain separate.
      </p>
    </>
  );
}

function RestoreReview({
  view,
  empty,
  statusKnown,
  onBack,
  onComplete,
}: {
  view: RestoreView;
  empty: boolean;
  statusKnown: boolean;
  onBack: () => void;
  onComplete: (r: RestoreReceipt) => void;
}) {
  const qc = useQueryClient();
  const navigate = useNavigate();
  const [checked, setChecked] = useState(false);
  const heading = useRef<HTMLHeadingElement>(null);
  useEffect(() => heading.current?.focus(), []);
  const restore = useMutation({
    mutationFn: () => commitRestore(view.id, view.checksum),
    onSuccess: onComplete,
    onError: () => {
      void qc.invalidateQueries({ queryKey: ["restore", view.id] });
      void qc.invalidateQueries({ queryKey: ["workspace"] });
    },
  });
  return (
    <section className="import-panel" aria-labelledby="restore-review-title">
      <p className="import-step">
        File integrity verified · Review before restoring
      </p>
      <h2 id="restore-review-title" ref={heading} tabIndex={-1}>
        Check your accounts and balances.
      </h2>
      <p className="muted">{view.filename}</p>
      <BackupSummary summary={view.summary} />
      {!statusKnown ? (
        <p className="input-hint">
          Waiting for workspace status before restoring.
        </p>
      ) : !empty ? (
        <div className="restore-existing-notice">
          <h3>This workspace already has records.</h3>
          <p>
            Restore into a new empty workspace to keep each set of records
            together. Create a new LedgerX login, then choose this same backup
            there.
          </p>
          <button
            className="text-button"
            onClick={() => {
              navigate("/new-workspace?next=restore");
            }}
          >
            Sign out and create a new workspace
            <Icon name="arrow" width="16" height="16" />
          </button>
        </div>
      ) : (
        <p className="input-hint">
          This workspace is empty. All records in this backup will be saved
          together. If saving fails, you can retry the same review safely.
        </p>
      )}
      <label className="import-confirm">
        <input
          type="checkbox"
          checked={checked}
          onChange={(e) => setChecked(e.target.checked)}
          disabled={restore.isPending}
        />
        <span>I reviewed the accounts and balances in this backup.</span>
      </label>
      {restore.isError && (
        <p className="form-error" role="alert">
          {apiError(restore.error)} Retrying the same review is safe.
        </p>
      )}
      <div className="import-actions">
        <button className="btn" onClick={onBack} disabled={restore.isPending}>
          Choose another backup
        </button>
        <button
          className="btn btn-primary"
          disabled={!empty || !checked || restore.isPending}
          onClick={() => restore.mutate()}
        >
          {restore.isPending ? "Restoring workspace…" : "Restore workspace"}
        </button>
      </div>
      <p className="input-hint">
        Review expires {new Date(view.expires_at).toLocaleString()}. A restore
        preserves recorded history and requires your current login for access.
      </p>
    </section>
  );
}

function RestoreComplete({
  receipt,
  onNew,
}: {
  receipt: RestoreReceipt;
  onNew: () => void;
}) {
  const qc = useQueryClient();
  const heading = useRef<HTMLHeadingElement>(null);
  useEffect(() => {
    heading.current?.focus();
    for (const a of receipt.accounts) {
      if (a.appearance) saveAppearance(a.id, a.appearance);
    }
    void qc.invalidateQueries({ queryKey: ["accounts"] });
    void refreshLedger(qc);
  }, [receipt.accounts, qc]);
  return (
    <section
      className="import-panel import-receipt"
      aria-labelledby="restore-complete-title"
    >
      <span className="import-receipt-icon">
        <Icon name="check" />
      </span>
      <h2 id="restore-complete-title" ref={heading} tabIndex={-1}>
        Your workspace is restored.
      </h2>
      <p className="muted">
        {receipt.filename} · Saved{" "}
        {new Date(receipt.completed_at).toLocaleString()}
      </p>
      <BackupSummary summary={receipt.summary} />
      <p>
        Accounts, financial records, corrections, and saved checks are ready.
        Your current login continues to control access.
      </p>
      <div className="import-actions">
        <Link className="btn btn-primary" to="/">
          Open overview
          <Icon name="arrow" width="16" height="16" />
        </Link>
        <Link className="btn" to="/activity">
          Review restored activity
        </Link>
        <button className="text-button" onClick={onNew}>
          Back to backup tools
        </button>
      </div>
      <p className="input-hint">
        This receipt is safe to reload. Reports can be generated from the
        restored records in Reports.
      </p>
    </section>
  );
}
