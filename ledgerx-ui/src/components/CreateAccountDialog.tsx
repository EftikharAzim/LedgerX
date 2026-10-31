import { useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { createAccount } from "../api/endpoints";
import { apiError } from "../lib/errors";
import {
  ACCOUNT_COLORS,
  ACCOUNT_ICONS,
  saveAppearance,
  type AccountAppearance,
} from "../lib/account-appearance";
import Icon from "./Icon";
import { useModalDialog } from "../lib/use-modal-dialog";

const CURRENCIES = [
  ["USD", "US Dollar"],
  ["EUR", "Euro"],
  ["GBP", "British Pound"],
  ["JPY", "Japanese Yen"],
  ["BDT", "Bangladeshi Taka"],
  ["INR", "Indian Rupee"],
];

export default function CreateAccountDialog({
  onClose,
  onCreated,
}: {
  onClose: () => void;
  onCreated: (name: string) => void;
}) {
  const dialog = useModalDialog();
  const qc = useQueryClient();
  const [name, setName] = useState("");
  const [currency, setCurrency] = useState("USD");
  const [appearance, setAppearance] = useState<AccountAppearance>({
    icon: "wallet",
    color: "sage",
  });
  const create = useMutation({
    mutationFn: () => createAccount(name.trim(), currency),
    onSuccess: (account) => {
      saveAppearance(account.id, appearance);
      qc.invalidateQueries({ queryKey: ["accounts"] });
      onCreated(account.name);
      onClose();
    },
  });

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (name.trim() && !create.isPending) create.mutate();
  };

  return (
    <dialog
      ref={dialog}
      className="account-dialog"
      aria-labelledby="create-account-title"
      aria-describedby="create-account-description"
      onCancel={(event) => {
        event.preventDefault();
        if (!create.isPending) onClose();
      }}
    >
      <div className="dialog-header">
        <span className={`account-symbol tone-${appearance.color}`}>
          <Icon name={appearance.icon} width="26" height="26" />
        </span>
        <button
          type="button"
          className="icon-button"
          aria-label="Close dialog"
          disabled={create.isPending}
          onClick={onClose}
        >
          <Icon name="close" />
        </button>
      </div>
      <h2 id="create-account-title">A place for your money.</h2>
      <p id="create-account-description" className="muted">
        Give your account a name and make it your own.
      </p>
      <form onSubmit={submit} className="account-form">
        <div>
          <label htmlFor="account-name" className="label">
            Account name
          </label>
          <input
            data-autofocus
            id="account-name"
            className="input"
            placeholder="e.g. Everyday spending"
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
            disabled={create.isPending}
          />
        </div>
        <div>
          <label htmlFor="account-currency" className="label">
            Currency
          </label>
          <select
            id="account-currency"
            className="input"
            value={currency}
            onChange={(e) => setCurrency(e.target.value)}
            disabled={create.isPending}
          >
            {CURRENCIES.map(([code, label]) => (
              <option key={code} value={code}>
                {code} — {label}
              </option>
            ))}
          </select>
          <p className="input-hint">
            Each account keeps its balance in one currency.
          </p>
        </div>
        <div className="appearance-options">
          <fieldset disabled={create.isPending}>
            <legend className="label">Choose an icon</legend>
            <div className="choice-row">
              {ACCOUNT_ICONS.map((icon) => (
                <label className="icon-choice" key={icon}>
                  <input
                    type="radio"
                    name="account-icon"
                    value={icon}
                    checked={appearance.icon === icon}
                    onChange={() => setAppearance({ ...appearance, icon })}
                  />
                  <span>
                    <Icon name={icon} />
                    <span className="sr-only">{icon}</span>
                  </span>
                </label>
              ))}
            </div>
          </fieldset>
          <fieldset disabled={create.isPending}>
            <legend className="label">Choose a color</legend>
            <div className="choice-row">
              {ACCOUNT_COLORS.map((color) => (
                <label className="color-choice" key={color}>
                  <input
                    type="radio"
                    name="account-color"
                    value={color}
                    checked={appearance.color === color}
                    onChange={() => setAppearance({ ...appearance, color })}
                  />
                  <span className={`tone-${color}`}>
                    <span className="sr-only">{color}</span>
                    {appearance.color === color && (
                      <Icon name="check" width="16" height="16" />
                    )}
                  </span>
                </label>
              ))}
            </div>
          </fieldset>
        </div>
        <p className="input-hint">
          Your icon and color are saved on this device.
        </p>
        {create.isError && (
          <p role="alert" className="form-error">
            <Icon name="info" />
            {apiError(
              create.error,
              "We couldn't create your account. Please try again.",
            )}
          </p>
        )}
        <div className="dialog-actions">
          <button
            type="button"
            className="btn"
            disabled={create.isPending}
            onClick={onClose}
          >
            Cancel
          </button>
          <button
            type="submit"
            className="btn btn-primary"
            disabled={!name.trim() || create.isPending}
          >
            {create.isPending ? "Creating account…" : "Create account"}
            <Icon name="arrow" width="17" height="17" />
          </button>
        </div>
      </form>
    </dialog>
  );
}
