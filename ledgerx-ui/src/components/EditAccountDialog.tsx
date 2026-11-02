import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { type Account, listAccounts, updateAccount } from "../api/endpoints";
import { apiError } from "../lib/errors";
import { refreshLedger } from "../lib/refresh-ledger";
import Dialog from "./Dialog";

export default function EditAccountDialog({
  account,
  onClose,
  onSaved,
}: {
  account: Account;
  onClose: () => void;
  onSaved: (message: string) => void;
}) {
  const qc = useQueryClient();
  // Keep the reviewed state until an explicit reload, even if a background
  // refetch discovers changes made in another tab.
  const [reviewed, setReviewed] = useState(account);
  const [name, setName] = useState(account.name);
  const [archiving, setArchiving] = useState(false);
  const [reloading, setReloading] = useState(false);
  const [reloadError, setReloadError] = useState("");
  const save = useMutation({
    mutationFn: (change: { name?: string; active?: boolean }) =>
      updateAccount(reviewed, change),
    onSuccess: (updated, change) => {
      qc.setQueryData<Account[]>(["accounts"], (items) =>
        items?.map((item) => (item.id === updated.id ? updated : item)),
      );
      void qc.invalidateQueries({ queryKey: ["accounts"] });
      void refreshLedger(qc);
      onSaved(
        change.active === false
          ? `${updated.name} archived.`
          : change.active === true
            ? `${updated.name} restored.`
            : "Account name updated.",
      );
    },
  });
  const error = save.isError ? apiError(save.error) : "";
  const stale = error.includes("account details changed");
  const schedules = error.includes("active recurring schedules");
  const busy = save.isPending || reloading;
  const validName =
    [...name.trim()].length >= 1 && [...name.trim()].length <= 80;
  const reload = async () => {
    setReloading(true);
    setReloadError("");
    try {
      const items = await listAccounts();
      const current = items.find((item) => item.id === reviewed.id);
      if (!current) throw new Error("This account is no longer available.");
      qc.setQueryData(["accounts"], items);
      setReviewed(current);
      setName(current.name);
      setArchiving(false);
      save.reset();
    } catch (err) {
      setReloadError(apiError(err, "Couldn't reload account details."));
    } finally {
      setReloading(false);
    }
  };
  return (
    <Dialog title="Edit account" onClose={onClose} busy={busy}>
      <form
        className="account-form"
        onSubmit={(event) => {
          event.preventDefault();
          if (!archiving && validName && name.trim() !== reviewed.name && !busy)
            save.mutate({ name: name.trim() });
        }}
      >
        <label className="label" htmlFor="edit-account-name">
          Account name
        </label>
        <input
          id="edit-account-name"
          data-autofocus
          className="input"
          value={name}
          onChange={(event) => setName(event.target.value)}
          disabled={busy || archiving}
          required
          aria-describedby="edit-account-currency"
        />
        <p id="edit-account-currency" className="input-hint">
          Currency: {reviewed.currency}. Create a separate account for another
          currency.
        </p>
        {[...name.trim()].length > 80 && (
          <p role="alert" className="form-error">
            Use 80 characters or fewer for the account name.
          </p>
        )}
        {archiving ? (
          <section
            className="account-archive-review"
            aria-labelledby="archive-account-title"
          >
            <h3 id="archive-account-title">Archive {reviewed.name}?</h3>
            <p>
              Its balance and history stay in your reports. Archiving does not
              move money.
            </p>
            <p>
              Restore it before adding entries, transferring money or confirming
              planned transactions. Existing plans remain available.
            </p>
            {name.trim() !== reviewed.name && (
              <p>Your unsaved name change will be discarded.</p>
            )}
          </section>
        ) : (
          <div className="account-availability">
            <p className="input-hint">
              {reviewed.active
                ? "Archive an account you no longer use. Its balance and history are kept."
                : "This account is archived. Restore it to record new activity. Recurring schedules stay paused or ended."}
            </p>
            <button
              type="button"
              className="text-button"
              disabled={busy}
              onClick={() => {
                save.reset();
                if (reviewed.active) setArchiving(true);
                else save.mutate({ active: true });
              }}
            >
              {save.isPending && save.variables.active === true
                ? "Restoring…"
                : reviewed.active
                  ? "Archive account"
                  : "Restore account"}
            </button>
          </div>
        )}
        {error && (
          <p className="form-error" role="alert">
            {error}
          </p>
        )}
        {schedules && (
          <Link className="text-link" to="/schedules">
            Manage recurring bills
          </Link>
        )}
        {stale && (
          <div>
            <p className="input-hint">
              Reloading replaces your unsaved name with the latest details.
            </p>
            <button
              type="button"
              className="text-button"
              disabled={busy}
              onClick={() => void reload()}
            >
              {reloading ? "Reloading…" : "Reload details"}
            </button>
          </div>
        )}
        {reloadError && (
          <p className="form-error" role="alert">
            {reloadError}
          </p>
        )}
        <div className="dialog-actions">
          <button
            type="button"
            className="btn"
            disabled={busy}
            onClick={() => {
              if (archiving) {
                setArchiving(false);
                save.reset();
              } else onClose();
            }}
          >
            {archiving ? "Back" : "Cancel"}
          </button>
          {archiving ? (
            <button
              type="button"
              className="btn btn-primary"
              disabled={busy || stale}
              onClick={() => save.mutate({ active: false })}
            >
              {save.isPending ? "Archiving…" : "Confirm archive"}
            </button>
          ) : (
            <button
              type="submit"
              className="btn btn-primary"
              disabled={
                busy || stale || !validName || name.trim() === reviewed.name
              }
            >
              {save.isPending && save.variables.name !== undefined
                ? "Saving…"
                : "Save name"}
            </button>
          )}
        </div>
      </form>
    </Dialog>
  );
}
