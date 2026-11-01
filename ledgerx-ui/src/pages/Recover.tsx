import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useMutation } from "@tanstack/react-query";
import { recoverAccess } from "../api/security";
import { endSession } from "../app/auth-session";
import { apiError } from "../lib/errors";

export default function Recover() {
  const nav = useNavigate();
  const [email, setEmail] = useState("");
  const [code, setCode] = useState("");
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [error, setError] = useState("");
  const recover = useMutation({
    mutationFn: () => recoverAccess(email, code, password),
    onSuccess: () => {
      endSession();
      nav("/login?recovered=1", { replace: true });
    },
  });
  return (
    <main className="recovery-page">
      <section className="import-panel" aria-labelledby="recover-title">
        <Link className="text-link" to="/login">
          Back to login
        </Link>
        <h1 id="recover-title">Recover access</h1>
        <p className="muted">
          Enter the recovery code you saved in Account & security. This resets
          your password and ends all existing sessions.
        </p>
        <form
          className="account-form"
          onSubmit={(event) => {
            event.preventDefault();
            setError("");
            if (password !== confirmation) {
              setError("The new passwords do not match.");
              return;
            }
            recover.mutate();
          }}
        >
          <label className="label" htmlFor="recover-email">
            Email address
          </label>
          <input
            id="recover-email"
            className="input"
            type="email"
            autoComplete="email"
            required
            value={email}
            onChange={(event) => setEmail(event.target.value)}
            disabled={recover.isPending}
          />
          <label className="label" htmlFor="recover-code">
            Saved recovery code
          </label>
          <input
            id="recover-code"
            className="input"
            type="password"
            autoComplete="off"
            required
            value={code}
            onChange={(event) => setCode(event.target.value)}
            disabled={recover.isPending}
          />
          <label className="label" htmlFor="recover-password">
            New password
          </label>
          <input
            id="recover-password"
            className="input"
            type="password"
            autoComplete="new-password"
            required
            minLength={8}
            maxLength={72}
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            disabled={recover.isPending}
          />
          <label className="label" htmlFor="recover-confirmation">
            Confirm new password
          </label>
          <input
            id="recover-confirmation"
            className="input"
            type="password"
            autoComplete="new-password"
            required
            minLength={8}
            maxLength={72}
            value={confirmation}
            onChange={(event) => setConfirmation(event.target.value)}
            disabled={recover.isPending}
          />
          <button className="btn btn-primary" disabled={recover.isPending}>
            {recover.isPending ? "Resetting…" : "Reset password"}
          </button>
          {(error || recover.isError) && (
            <p role="alert" className="form-error">
              {error || apiError(recover.error)}
            </p>
          )}
        </form>
        <p className="input-hint">
          Without a saved recovery code, LedgerX cannot reset your password. A
          financial backup can be restored into a new empty workspace, but it
          does not recover the old login.
        </p>
      </section>
    </main>
  );
}
