import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useMutation, useQuery } from "@tanstack/react-query";
import {
  getSecurityState,
  createRecoveryCode,
  changePassword,
  deleteAccount,
} from "../api/security";
import { endSession } from "../app/auth-session";
import { apiError } from "../lib/errors";

export default function Security() {
  const nav = useNavigate();
  const state = useQuery({ queryKey: ["security"], queryFn: getSecurityState });
  const [codePassword, setCodePassword] = useState("");
  const [code, setCode] = useState("");
  const [oldPassword, setOldPassword] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [deletePassword, setDeletePassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [showDeletion, setShowDeletion] = useState(false);
  const [localError, setLocalError] = useState("");
  const recovery = useMutation({
    mutationFn: () => createRecoveryCode(codePassword),
    onSuccess: (result) => {
      setCode(result.recovery_code);
      setCodePassword("");
      void state.refetch();
    },
  });
  const change = useMutation({
    mutationFn: () => changePassword(oldPassword, password),
    onSuccess: () => {
      endSession();
      nav("/login?password_changed=1", { replace: true });
    },
  });
  const remove = useMutation({
    mutationFn: () => deleteAccount(deletePassword, confirmation),
    onSuccess: () => {
      endSession();
      nav("/login?deleted=1", { replace: true });
    },
  });
  const busy = recovery.isPending || change.isPending || remove.isPending;

  function downloadCode() {
    const blob = new Blob(
      [
        `LedgerX recovery code\nEmail: ${state.data?.email ?? ""}\nCode: ${code}\n\nKeep this private and separate from your password. The code works once.\n`,
      ],
      { type: "text/plain" },
    );
    const url = URL.createObjectURL(blob),
      link = document.createElement("a");
    link.href = url;
    link.download = "ledgerx-recovery-code.txt";
    document.body.append(link);
    link.click();
    link.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }

  return (
    <div className="workspace-tools-page">
      <div className="page-heading">
        <div>
          <h1>Account & security</h1>
          <p className="muted">Protect your access and control your data.</p>
        </div>
      </div>
      {state.isPending && <p role="status">Loading account settings…</p>}
      {state.isError && (
        <p role="alert" className="form-error">
          {apiError(state.error)}{" "}
          <button className="text-button" onClick={() => state.refetch()}>
            Try again
          </button>
        </p>
      )}
      {state.data && (
        <>
          <p className="muted">Signed in as {state.data.email}</p>
          <div className="workspace-tools-grid">
            <section className="import-panel" aria-labelledby="recovery-title">
              <h2 id="recovery-title">Save a recovery code</h2>
              <p className="muted">
                Use this private code if you forget your password. LedgerX does
                not send password reset emails. Save a code before you need it.
              </p>
              <p className="input-hint">
                {state.data.recovery_enabled
                  ? "A recovery code is configured. Generating another replaces it immediately."
                  : "You have not configured a recovery code yet."}
              </p>
              {code ? (
                <div className="account-form">
                  <p role="status">
                    Your new code is ready. It will only be shown here once.
                  </p>
                  <label className="label" htmlFor="recovery-code">
                    Recovery code
                  </label>
                  <textarea
                    id="recovery-code"
                    className="input recovery-code"
                    readOnly
                    rows={3}
                    value={code}
                    spellCheck={false}
                    onFocus={(event) => event.currentTarget.select()}
                  />
                  <p className="input-hint">
                    Keep it separate from your password. Anyone with this code
                    and your email can reset access. It works once; generate a
                    new one after recovery.
                  </p>
                  <div className="import-actions">
                    <button className="btn btn-primary" onClick={downloadCode}>
                      Download recovery code
                    </button>
                    <button className="btn" onClick={() => setCode("")}>
                      I saved my code
                    </button>
                  </div>
                </div>
              ) : (
                <form
                  className="account-form"
                  onSubmit={(event) => {
                    event.preventDefault();
                    recovery.mutate();
                  }}
                >
                  <label className="label" htmlFor="code-password">
                    Current password for recovery code
                  </label>
                  <input
                    id="code-password"
                    className="input"
                    type="password"
                    autoComplete="current-password"
                    required
                    maxLength={72}
                    value={codePassword}
                    onChange={(event) => setCodePassword(event.target.value)}
                    disabled={busy}
                  />
                  <button className="btn btn-primary" disabled={busy}>
                    {recovery.isPending
                      ? "Generating…"
                      : state.data.recovery_enabled
                        ? "Replace recovery code"
                        : "Generate recovery code"}
                  </button>
                  {recovery.isError && (
                    <p className="form-error" role="alert">
                      {apiError(recovery.error)} If the response was
                      interrupted, generate a new code; the previous one may
                      have been replaced.
                    </p>
                  )}
                </form>
              )}
            </section>
            <section className="import-panel" aria-labelledby="password-title">
              <h2 id="password-title">Change password</h2>
              <p className="muted">
                Changing your password ends every signed-in session. Your saved
                recovery code keeps working.
              </p>
              <form
                className="account-form"
                onSubmit={(event) => {
                  event.preventDefault();
                  setLocalError("");
                  if (password !== confirmPassword) {
                    setLocalError("The new passwords do not match.");
                    return;
                  }
                  change.mutate();
                }}
              >
                <label className="label" htmlFor="old-password">
                  Current password
                </label>
                <input
                  id="old-password"
                  className="input"
                  type="password"
                  autoComplete="current-password"
                  required
                  maxLength={72}
                  value={oldPassword}
                  onChange={(event) => setOldPassword(event.target.value)}
                  disabled={busy}
                />
                <label className="label" htmlFor="new-password">
                  New password
                </label>
                <input
                  id="new-password"
                  className="input"
                  type="password"
                  autoComplete="new-password"
                  required
                  minLength={8}
                  maxLength={72}
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  disabled={busy}
                />
                <label className="label" htmlFor="confirm-new-password">
                  Confirm new password
                </label>
                <input
                  id="confirm-new-password"
                  className="input"
                  type="password"
                  autoComplete="new-password"
                  required
                  minLength={8}
                  maxLength={72}
                  value={confirmPassword}
                  onChange={(event) => setConfirmPassword(event.target.value)}
                  disabled={busy}
                />
                <button className="btn" disabled={busy}>
                  {change.isPending
                    ? "Changing…"
                    : "Change password and log out"}
                </button>
                {(localError || change.isError) && (
                  <p className="form-error" role="alert">
                    {localError || apiError(change.error)}
                  </p>
                )}
              </form>
            </section>
          </div>
          <section
            className="import-panel security-delete"
            aria-labelledby="delete-title"
          >
            <h2 id="delete-title">Delete account</h2>
            <p className="muted">
              Permanently remove your account and live financial records.
              Generated reports and retained events are queued for background
              deletion. Derived summaries expire within five minutes. Downloaded
              copies and server backups are separate.
            </p>
            <p>
              <Link className="text-link" to="/workspace">
                Download a financial backup first
              </Link>
            </p>
            {!showDeletion ? (
              <button
                className="text-button"
                onClick={() => setShowDeletion(true)}
              >
                Review account deletion
              </button>
            ) : (
              <form
                className="account-form"
                onSubmit={(event) => {
                  event.preventDefault();
                  remove.mutate();
                }}
              >
                <p>
                  This cannot be undone. It removes every account, transaction,
                  category, import, and saved balance check in this workspace.
                </p>
                <label className="label" htmlFor="delete-password">
                  Password to delete this account
                </label>
                <input
                  id="delete-password"
                  className="input"
                  type="password"
                  autoComplete="current-password"
                  required
                  maxLength={72}
                  value={deletePassword}
                  onChange={(event) => setDeletePassword(event.target.value)}
                  disabled={busy}
                />
                <label className="label" htmlFor="delete-confirmation">
                  Type DELETE to confirm
                </label>
                <input
                  id="delete-confirmation"
                  className="input"
                  required
                  pattern="DELETE"
                  autoComplete="off"
                  value={confirmation}
                  onChange={(event) => setConfirmation(event.target.value)}
                  disabled={busy}
                />
                <div className="import-actions">
                  <button
                    className="btn"
                    disabled={busy || confirmation !== "DELETE"}
                  >
                    {remove.isPending
                      ? "Deleting…"
                      : "Permanently delete my account"}
                  </button>
                  <button
                    className="btn"
                    type="button"
                    disabled={busy}
                    onClick={() => {
                      setShowDeletion(false);
                      setDeletePassword("");
                      setConfirmation("");
                      remove.reset();
                    }}
                  >
                    Cancel
                  </button>
                </div>
                {remove.isError && (
                  <p className="form-error" role="alert">
                    {apiError(remove.error)}
                  </p>
                )}
              </form>
            )}
          </section>
        </>
      )}
    </div>
  );
}
