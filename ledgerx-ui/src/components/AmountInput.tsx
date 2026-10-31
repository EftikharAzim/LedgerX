import { useState } from "react";
import { currencyDigits, parseAmount } from "../lib/money";

export default function AmountInput({
  value,
  onChange,
  currency,
  disabled = false,
}: {
  value: string;
  onChange: (value: string) => void;
  currency: string;
  disabled?: boolean;
}) {
  const [touched, setTouched] = useState(false);
  const digits = currencyDigits(currency);
  const invalid = touched && !!value && parseAmount(value, currency) === null;
  return (
    <div>
      <label htmlFor="money-amount" className="label">
        Amount
      </label>
      <div className={`amount-input ${invalid ? "invalid" : ""}`}>
        <input
          id="money-amount"
          data-autofocus
          type="text"
          inputMode={digits ? "decimal" : "numeric"}
          autoComplete="off"
          value={value}
          onChange={(e) => onChange(e.target.value)}
          onBlur={() => setTouched(true)}
          disabled={disabled}
          required
          placeholder={digits ? `0.${"0".repeat(digits)}` : "0"}
          aria-invalid={invalid}
          aria-describedby="amount-help"
          maxLength={24}
        />
        <span>{currency}</span>
      </div>
      <p
        id="amount-help"
        className={invalid ? "field-error amount-help" : "input-hint"}
      >
        {invalid
          ? `Enter a positive amount${digits ? ` with up to ${digits} decimal places` : " in whole units"}.`
          : digits
            ? `Use a dot for decimals. Up to ${digits} decimal places.`
            : "This currency uses whole units, without decimals."}
      </p>
    </div>
  );
}
