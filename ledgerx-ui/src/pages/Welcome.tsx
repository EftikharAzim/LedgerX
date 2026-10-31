import { useState } from "react";
import { Link } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { listAccounts, listEntries } from "../api/endpoints";
import CreateAccountDialog from "../components/CreateAccountDialog";
import TransactionDialog from "../components/TransactionDialog";
import Icon from "../components/Icon";

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
    queryFn: () => listEntries(account!.id, undefined, 1),
    enabled: !!account,
  });
  const hasEntry = !!activity.data?.entries.length;
  const complete = (account ? 1 : 0) + (hasEntry ? 1 : 0);
  const pending = accountsQuery.isPending;

  return (
    <div className="welcome-page">
      <div className="page-heading">
        <div>
          <p className="eyebrow">A GOOD PLACE TO BEGIN</p>
          <h1>
            A little setup.
            <br />
            <em>A lot of possibility.</em>
          </h1>
          <p className="page-description">
            Two small steps to make this workspace feel like yours.
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
      <div className="welcome-grid">
        <section className="welcome-steps" aria-labelledby="setup-title">
          <div className="welcome-progress-heading">
            <h2 id="setup-title">Make yourself at home</h2>
            <span>
              {pending ? "Getting ready…" : `${complete} of 2 complete`}
            </span>
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
              <span>We couldn't check your accounts.</span>
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
                  <h3>Give your money a place.</h3>
                  <p>
                    Create an account for everyday spending, a future goal, or
                    anything in between.
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
                  <h3>Start with one entry.</h3>
                  <p>
                    Record an income or expense. A small beginning is all it
                    takes.
                  </p>
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
                      Your account has its first entry.
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
              <h3>
                {complete === 2
                  ? "You're ready to find your rhythm."
                  : "Go at your own pace."}
              </h3>
              <p>
                {complete === 2
                  ? "Your overview brings it all together."
                  : "You can come back to these steps whenever you like."}
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
        <aside className="welcome-note">
          <Icon name="sparkle" width="38" height="38" />
          <p className="eyebrow">A LITTLE PERSPECTIVE</p>
          <h2>
            Different goals.
            <br />
            <em>One clearer picture.</em>
          </h2>
          <p>
            An account is a place to organize your records. Choose its currency,
            add your entries, and see how the balance comes together.
          </p>
          <div>
            <Icon name="wallet" width="18" height="18" />
            <span>Organize by purpose.</span>
          </div>
          <div>
            <Icon name="tag" width="18" height="18" />
            <span>Add context with categories.</span>
          </div>
          <div>
            <Icon name="export" width="18" height="18" />
            <span>Take a monthly CSV with you.</span>
          </div>
          <p className="welcome-note-foot">
            The first step doesn't have to be a big one.
          </p>
        </aside>
      </div>
      {dialog === "account" && (
        <CreateAccountDialog
          onClose={() => setDialog(null)}
          onCreated={(name, created) => {
            setChoice(created.id);
            setMessage(`${name} is ready. Let's give it a first entry.`);
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
            void Promise.all([
              qc.invalidateQueries({ queryKey: ["welcome-activity"] }),
              qc.invalidateQueries({ queryKey: ["entries"] }),
              qc.invalidateQueries({ queryKey: ["balance"] }),
              qc.invalidateQueries({ queryKey: ["summary"] }),
            ]);
          }}
        />
      )}
    </div>
  );
}
