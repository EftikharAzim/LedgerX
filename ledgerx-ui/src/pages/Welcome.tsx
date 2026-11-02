import { useState } from "react";
import { Link } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { listAccounts, listEntries } from "../api/endpoints";
import CreateAccountDialog from "../components/CreateAccountDialog";
import TransactionDialog from "../components/TransactionDialog";
import Icon from "../components/Icon";
import { refreshLedger } from "../lib/refresh-ledger";

export default function Welcome() {
  const qc = useQueryClient();
  const accountsQuery = useQuery({
    queryKey: ["accounts"],
    queryFn: listAccounts,
  });
  const accounts = accountsQuery.data?.filter((a) => a.kind === "normal") ?? [];
  const [choice, setChoice] = useState<number | null>(null);
  const [dialog, setDialog] = useState<"account" | "transaction" | null>(null);
  const [message, setMessage] = useState("");
  const account =
    accounts.find((a) => a.id === choice) ??
    accounts.find((a) => a.active) ??
    accounts[0];
  const activity = useQuery({
    queryKey: ["welcome-activity", account?.id],
    queryFn: () => listEntries(account!.id, undefined, 20),
    enabled: !!account,
  });
  const hasEntry = !!activity.data?.entries.some(
    (entry) => entry.entry_type !== "opening_balance",
  );
  const complete = (account ? 1 : 0) + (hasEntry ? 1 : 0);
  const pending = accountsQuery.isPending;

  return (
    <div className="welcome-page">
      <div className="page-heading">
        <div>
          <h1>Getting started</h1>
          <p className="page-description">
            Add an account and starting balance, then record your first
            purchase.
          </p>
        </div>
        <Link className="text-link" to="/">
          Skip for now
          <Icon name="arrow" width="16" height="16" />
        </Link>
      </div>
      {message && (
        <p role="status" className="success-notice">
          <Icon name="check" />
          {message}
        </p>
      )}
      <section
        className="import-panel welcome-start-options"
        aria-labelledby="start-options-title"
      >
        <h2 id="start-options-title">Choose how to start</h2>
        <div className="import-actions">
          <a className="btn" href="#setup-title">
            Start fresh
          </a>
          <Link className="btn" to="/imports">
            Import a CSV
          </Link>
          <Link className="btn" to="/workspace">
            Restore a backup
          </Link>
        </div>
        <p className="input-hint">
          <Link className="text-link" to="/security">
            Save a recovery code
          </Link>{" "}
          so you can reset access if you forget your password.
        </p>
      </section>
      <div className="welcome-grid">
        <section className="welcome-steps" aria-labelledby="setup-title">
          <div className="welcome-progress-heading">
            <h2 id="setup-title">Set up tracking</h2>
            <span>{pending ? "Loading…" : `${complete} of 2 complete`}</span>
          </div>
          <div
            className="setup-progress"
            role="progressbar"
            aria-label="Setup progress"
            aria-valuenow={complete}
            aria-valuemin={0}
            aria-valuemax={2}
          >
            <span style={{ width: `${complete * 50}%` }} />
          </div>
          {accountsQuery.isError ? (
            <div role="alert" className="error-notice">
              <span>Couldn't load your accounts.</span>
              <button
                className="text-button"
                onClick={() => accountsQuery.refetch()}
              >
                Try again
              </button>
            </div>
          ) : pending ? (
            <div
              className="setup-loading"
              aria-busy="true"
              aria-label="Loading setup"
            >
              <span className="skeleton" />
              <span className="skeleton" />
            </div>
          ) : (
            <>
              <div className={`setup-step ${account ? "complete" : "current"}`}>
                <span className="setup-step-number">
                  {account ? (
                    <Icon name="check" width="17" height="17" />
                  ) : (
                    "01"
                  )}
                </span>
                <div>
                  <h3>Create an account</h3>
                  <p>
                    Use a separate account for each bank account, cash balance
                    or mobile wallet.
                  </p>
                  {account ? (
                    <div className="setup-account-choice">
                      <label htmlFor="setup-account">Your account</label>
                      <select
                        id="setup-account"
                        value={account.id}
                        onChange={(e) => setChoice(Number(e.target.value))}
                      >
                        {accounts.map((a) => (
                          <option key={a.id} value={a.id}>
                            {a.name} · {a.currency}
                          </option>
                        ))}
                      </select>
                      <button
                        className="text-button"
                        onClick={() => setDialog("account")}
                      >
                        Create another
                      </button>
                    </div>
                  ) : (
                    <button
                      className="btn btn-primary"
                      onClick={() => setDialog("account")}
                    >
                      <Icon name="plus" width="17" height="17" />
                      Create your first account
                    </button>
                  )}
                </div>
              </div>
              <div
                className={`setup-step ${hasEntry ? "complete" : account ? "current" : "upcoming"}`}
              >
                <span className="setup-step-number">
                  {hasEntry ? (
                    <Icon name="check" width="17" height="17" />
                  ) : (
                    "02"
                  )}
                </span>
                <div>
                  <h3>Record a transaction</h3>
                  <p>Enter an amount, date and optional category.</p>
                  {account && activity.isPending ? (
                    <p className="setup-checking" role="status">
                      Checking your account activity…
                    </p>
                  ) : account && activity.isError ? (
                    <p role="alert" className="form-error">
                      Couldn't check your activity.{" "}
                      <button
                        className="text-button"
                        onClick={() => activity.refetch()}
                      >
                        Try again
                      </button>
                    </p>
                  ) : hasEntry ? (
                    <p className="setup-done">
                      <Icon name="check" width="15" height="15" />
                      First transaction recorded.
                    </p>
                  ) : (
                    <button
                      className="btn"
                      disabled={
                        !account?.active ||
                        activity.isPending ||
                        activity.isError
                      }
                      onClick={() => setDialog("transaction")}
                    >
                      <Icon name="receipt" width="17" height="17" />
                      Record first transaction
                    </button>
                  )}
                  {account && !account.active && !hasEntry && (
                    <p className="input-hint">
                      Choose an active account or create another to record an
                      entry.
                    </p>
                  )}
                </div>
              </div>
            </>
          )}
          <div className={`setup-finish ${complete === 2 ? "ready" : ""}`}>
            <div>
              <h3>{complete === 2 ? "Setup complete" : "Setup in progress"}</h3>
              <p>
                {complete === 2
                  ? "Review your balances and transactions in Overview."
                  : "You can finish these steps later."}
              </p>
            </div>
            <Link
              className={`btn ${complete === 2 ? "btn-primary" : ""}`}
              to="/"
            >
              Go to overview
              <Icon name="arrow" width="17" height="17" />
            </Link>
          </div>
        </section>
      </div>
      {dialog === "account" && (
        <CreateAccountDialog
          onClose={() => setDialog(null)}
          onCreated={(name, created) => {
            setChoice(created.id);
            setMessage(`${name} created.`);
            setDialog(null);
          }}
        />
      )}
      {dialog === "transaction" && account?.active && (
        <TransactionDialog
          account={account}
          onClose={() => setDialog(null)}
          onSaved={(notice) => {
            setMessage(notice);
            setDialog(null);
            void refreshLedger(qc);
          }}
        />
      )}
    </div>
  );
}
