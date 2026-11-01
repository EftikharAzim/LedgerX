import { downloadBlob } from "../lib/download";
import { useEffect, useRef, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { listAccounts, formatMoney, type Account } from "../api/endpoints";
import {
  inspectCSV,
  getImportContext,
  previewImport,
  getImport,
  listImports,
  commitImport,
  type CSVInspection,
  type ImportMapping,
  type ImportView,
  type ImportPreview,
  type ImportReceipt,
} from "../api/imports";
import {
  MAX_CSV_BYTES,
  decodeCSV,
  guessMapping,
  mappingError,
  minorInput,
  readyRows,
  importProjection,
} from "../lib/imports";
import { parseStartingBalance } from "../lib/money";
import { apiError } from "../lib/errors";
import { refreshLedger } from "../lib/refresh-ledger";
import Icon from "../components/Icon";
import CreateAccountDialog from "../components/CreateAccountDialog";
import BalanceCheckDialog from "../components/BalanceCheckDialog";

type Draft = {
  csv: string;
  filename: string;
  doc: CSVInspection;
  accountID: number;
  sourceGroup?: string;
};

function useStepHeading() {
  const ref = useRef<HTMLHeadingElement>(null);
  useEffect(() => {
    ref.current?.focus();
  }, []);
  return ref;
}

export default function Imports() {
  const qc = useQueryClient();
  const [params, setParams] = useSearchParams();
  const id = Number(params.get("import_id")) || 0;
  const [version, setVersion] = useState(0);
  const accounts = useQuery({ queryKey: ["accounts"], queryFn: listAccounts });
  const recent = useQuery({ queryKey: ["imports"], queryFn: listImports });
  const staged = useQuery({
    queryKey: ["import", id],
    queryFn: () => getImport(id),
    enabled: id > 0,
    retry: false,
  });
  const normal = accounts.data?.filter((a) => a.kind === "normal") ?? [];
  const show = (view: ImportView) => {
    qc.setQueryData(["import", view.id], view);
    setParams({ import_id: String(view.id) });
    void qc.invalidateQueries({ queryKey: ["imports"] });
  };
  const anotherSource = () => {
    setParams({});
    void qc.invalidateQueries({ queryKey: ["import-context"] });
  };
  const startAgain = () => {
    setVersion((n) => n + 1);
    setParams({});
  };
  return (
    <div className="imports-page">
      <div className="page-heading">
        <div>
          <h1>Import activity</h1>
          <p className="muted">
            Bring your spending history with you. Review everything before
            saving.
          </p>
        </div>
        <Link className="text-link" to="/activity">
          Back to activity
          <Icon name="arrow" width="16" height="16" />
        </Link>
      </div>
      {accounts.isPending ? (
        <p role="status">Loading accounts…</p>
      ) : accounts.isError ? (
        <p className="form-error" role="alert">
          Couldn't load accounts.{" "}
          <button className="text-button" onClick={() => accounts.refetch()}>
            Try again
          </button>
        </p>
      ) : (
        <>
          <div hidden={id > 0}>
            <ImportWizard
              key={version}
              accounts={normal.filter((a) => a.active)}
              initialAccount={Number(params.get("account_id")) || 0}
              onPreview={show}
            />
          </div>
          {id > 0 &&
            (staged.isPending ? (
              <p role="status">Loading saved review…</p>
            ) : staged.isError ? (
              <div className="import-panel">
                <p role="alert" className="form-error">
                  {apiError(staged.error)}
                </p>
                <div className="import-actions">
                  <button className="btn" onClick={() => staged.refetch()}>
                    Try loading again
                  </button>
                  <button className="text-button" onClick={startAgain}>
                    Choose file again
                  </button>
                </div>
              </div>
            ) : staged.data?.receipt ? (
              <Receipt
                receipt={staged.data.receipt}
                account={normal.find((a) => a.id === staged.data.account_id)}
                onNew={startAgain}
                onAnotherSource={anotherSource}
              />
            ) : staged.data?.preview ? (
              <Review
                key={id}
                view={staged.data}
                preview={staged.data.preview}
                onEdit={() => {
                  setParams({});
                  void qc.invalidateQueries({ queryKey: ["import-context"] });
                }}
                onComplete={(receipt) =>
                  show({
                    ...staged.data!,
                    status: "completed",
                    preview: undefined,
                    receipt,
                  })
                }
              />
            ) : null)}
        </>
      )}
      <details className="import-history">
        <summary>Recent imports and saved reviews</summary>
        <p className="input-hint">
          Reviews expire after 24 hours. Completed import receipts remain
          available.
        </p>
        {recent.isPending ? (
          <p>Loading…</p>
        ) : recent.isError ? (
          <p role="alert">
            Couldn't load imports.{" "}
            <button className="text-button" onClick={() => recent.refetch()}>
              Try again
            </button>
          </p>
        ) : !recent.data?.length ? (
          <p className="muted">No imports yet.</p>
        ) : (
          <ul>
            {recent.data.map((item) => (
              <li key={item.id}>
                <Link to={`/imports?import_id=${item.id}`}>
                  <strong>{item.filename}</strong>
                  <span>
                    {item.status === "completed"
                      ? `${item.receipt?.imported_count ?? 0} imported`
                      : "Saved review"}{" "}
                    · {new Date(item.created_at).toLocaleDateString()}
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

function ImportWizard({
  accounts,
  initialAccount,
  onPreview,
}: {
  accounts: Account[];
  initialAccount: number;
  onPreview: (view: ImportView) => void;
}) {
  const [accountID, setAccountID] = useState(
    initialAccount || accounts[0]?.id || 0,
  );
  const [file, setFile] = useState<File | null>(null);
  const [delimiter, setDelimiter] = useState("auto");
  const [draft, setDraft] = useState<Draft | null>(null);
  const [create, setCreate] = useState(false);
  const inspect = useMutation({
    mutationFn: async () => {
      if (!file) throw new Error("Choose a CSV file.");
      if (file.size > MAX_CSV_BYTES)
        throw new Error("Choose a CSV no larger than 16 MiB.");
      const csv = decodeCSV(new Uint8Array(await file.arrayBuffer()));
      const doc = await inspectCSV(csv, delimiter);
      return {
        csv,
        filename: file.name,
        doc,
        accountID:
          doc.source_groups?.length === 1 && doc.source_groups[0].currency
            ? ((
                accounts.find(
                  (a) =>
                    a.id === accountID &&
                    a.currency === doc.source_groups![0].currency,
                ) ??
                accounts.find(
                  (a) => a.currency === doc.source_groups![0].currency,
                )
              )?.id ?? 0)
            : accountID,
        sourceGroup:
          doc.source_groups?.length === 1
            ? doc.source_groups[0].key
            : undefined,
      };
    },
    onSuccess: setDraft,
  });
  const account = accounts.find(
    (a) => a.id === (draft?.accountID ?? accountID),
  );
  if (draft) {
    const groups = draft.doc.source_groups ?? [];
    const group = groups.find((g) => g.key === draft.sourceGroup);
    const needsSource = groups.length > 1 && !group;
    const destinations = accounts.filter(
      (a) => !group?.currency || a.currency === group.currency,
    );
    return (
      <>
        {(groups.length > 1 || !account) && (
          <section
            className="import-panel"
            aria-labelledby="import-routing-title"
          >
            <h2 id="import-routing-title">Choose an account from this file</h2>
            <p className="muted">
              {draft.doc.row_count.toLocaleString()} records in this file.
              Import each wallet into an account with the same currency. You can
              reuse this file for the other wallets.
            </p>
            {groups.length > 1 && (
              <>
                <label className="label" htmlFor="import-source-group">
                  Source account and currency
                </label>
                <select
                  id="import-source-group"
                  className="input"
                  value={draft.sourceGroup ?? ""}
                  onChange={(e) => {
                    const g = groups.find((x) => x.key === e.target.value);
                    const target =
                      accounts.find(
                        (a) =>
                          a.id === draft.accountID &&
                          (!g?.currency || a.currency === g.currency),
                      ) ??
                      accounts.find(
                        (a) => !g?.currency || a.currency === g.currency,
                      );
                    setDraft({
                      ...draft,
                      sourceGroup: e.target.value,
                      accountID: target?.id ?? 0,
                    });
                  }}
                >
                  <option value="">Choose a source account</option>
                  {groups.map((g) => (
                    <option key={g.key} value={g.key}>
                      {g.account || "Source wallet"} ·{" "}
                      {g.currency || "Currency not specified"} ·{" "}
                      {g.rows.toLocaleString()} records
                    </option>
                  ))}
                </select>
              </>
            )}
            {!needsSource && (
              <>
                <label className="label" htmlFor="import-destination">
                  Destination account
                </label>
                <select
                  id="import-destination"
                  className="input"
                  value={draft.accountID || ""}
                  onChange={(e) =>
                    setDraft({ ...draft, accountID: Number(e.target.value) })
                  }
                >
                  <option value="">Choose an account</option>
                  {destinations.map((a) => (
                    <option key={a.id} value={a.id}>
                      {a.name} · {a.currency}
                    </option>
                  ))}
                </select>
                <button className="text-button" onClick={() => setCreate(true)}>
                  Create destination account
                </button>
              </>
            )}
            <button className="text-button" onClick={() => setDraft(null)}>
              Choose a different file
            </button>
          </section>
        )}
        {!needsSource && account && (
          <MappingStep
            key={`${draft.accountID}-${draft.sourceGroup ?? ""}`}
            draft={draft}
            account={account}
            onBack={() => setDraft(null)}
            onPreview={onPreview}
          />
        )}
        {create && (
          <CreateAccountDialog
            initialCurrency={group?.currency || "USD"}
            onClose={() => setCreate(false)}
            onCreated={(_name, a) => {
              setCreate(false);
              setDraft({ ...draft, accountID: a.id });
            }}
          />
        )}
      </>
    );
  }
  return (
    <section className="import-panel" aria-labelledby="import-file-title">
      <p className="import-step">Step 1 of 3 · Choose a file</p>
      <h2 id="import-file-title">Bring your full history.</h2>
      <p className="muted">
        Use a bank or spreadsheet export with one row per income, expense, or
        refund. Transfers and full LedgerX reports need separate handling.
      </p>
      <form
        className="account-form"
        onSubmit={(e) => {
          e.preventDefault();
          if (accountID && file) inspect.mutate();
        }}
      >
        <div>
          <label className="label" htmlFor="import-account">
            Import into
          </label>
          <select
            id="import-account"
            className="input"
            value={accountID}
            onChange={(e) => {
              setAccountID(Number(e.target.value));
              inspect.reset();
            }}
            required
            disabled={inspect.isPending}
          >
            <option value="" disabled>
              Choose an account
            </option>
            {accounts.map((a) => (
              <option key={a.id} value={a.id}>
                {a.name} · {a.currency}
              </option>
            ))}
          </select>
          <button
            type="button"
            className="text-button"
            onClick={() => setCreate(true)}
            disabled={inspect.isPending}
          >
            Create an account
          </button>
        </div>
        <div>
          <label className="label" htmlFor="import-file">
            CSV file
          </label>
          <input
            id="import-file"
            className="input"
            type="file"
            accept=".csv,text/csv,text/tab-separated-values"
            required
            disabled={inspect.isPending}
            onChange={(e) => {
              setFile(e.target.files?.[0] ?? null);
              inspect.reset();
            }}
          />
          <p className="input-hint">
            Up to 50,000 records and 16 MiB, with a header row. UTF-8 and UTF-16
            with a byte-order mark are supported. Separators are detected
            automatically. Your original file is not stored.
          </p>
        </div>
        <details>
          <summary>File separator</summary>
          <label className="sr-only" htmlFor="import-delimiter">
            File separator
          </label>
          <select
            id="import-delimiter"
            className="input"
            value={delimiter}
            disabled={inspect.isPending}
            onChange={(e) => {
              setDelimiter(e.target.value);
              inspect.reset();
            }}
          >
            <option value="auto">Detect automatically</option>
            <option value="comma">Comma</option>
            <option value="semicolon">Semicolon</option>
            <option value="tab">Tab</option>
          </select>
        </details>
        {inspect.isError && (
          <p className="form-error" role="alert">
            {apiError(
              inspect.error,
              "Couldn't read this file. Save it as UTF-8 CSV and try again.",
            )}
          </p>
        )}
        <div className="import-actions">
          <button
            className="btn btn-primary"
            disabled={!file || !accountID || inspect.isPending}
          >
            {inspect.isPending ? "Reading file…" : "Continue"}
            <Icon name="arrow" width="16" height="16" />
          </button>
        </div>
      </form>
      <details className="import-help">
        <summary>Need an example?</summary>
        <p>
          Start with columns named Date, Amount, Description, Category, and
          Currency. Use negative amounts for expenses and positive amounts for
          income.
        </p>
        <button
          className="text-button"
          onClick={() => downloadTemplate(account?.currency ?? "USD")}
        >
          Download example CSV
        </button>
        <p>
          Transfers move money between your own accounts. Record them using
          Transfer to avoid counting them as income or spending.
        </p>
      </details>
      {create && (
        <CreateAccountDialog
          onClose={() => setCreate(false)}
          onCreated={(_name, a) => setAccountID(a.id)}
        />
      )}
    </section>
  );
}

function downloadTemplate(currency: string) {
  const text = `Date,Amount,Description,Category,Currency\n2024-01-02,-12,Coffee,Food,${currency}\n2024-01-03,100,Salary,Income,${currency}\n`;
  downloadBlob(
    new Blob([text], { type: "text/csv;charset=utf-8" }),
    "ledgerx-import-example.csv",
  );
}

function MappingStep({
  draft,
  account,
  onBack,
  onPreview,
}: {
  draft: Draft;
  account: Account;
  onBack: () => void;
  onPreview: (v: ImportView) => void;
}) {
  const context = useQuery({
    queryKey: ["import-context", account.id],
    queryFn: () => getImportContext(account.id),
  });
  if (context.isPending) return <p role="status">Reading account balance…</p>;
  if (context.isError)
    return (
      <div className="import-panel">
        <p role="alert" className="form-error">
          {apiError(context.error)}
        </p>
        <button className="btn" onClick={() => context.refetch()}>
          Try again
        </button>
        <button className="text-button" onClick={onBack}>
          Back
        </button>
      </div>
    );
  return (
    <MappingForm
      draft={draft}
      account={account}
      context={context.data}
      onBack={onBack}
      onPreview={onPreview}
    />
  );
}

function MappingForm({
  draft,
  account,
  context,
  onBack,
  onPreview,
}: {
  draft: Draft;
  account: Account;
  context: Awaited<ReturnType<typeof getImportContext>>;
  onBack: () => void;
  onPreview: (v: ImportView) => void;
}) {
  const heading = useStepHeading();
  const [mapping, setMapping] = useState(() => guessMapping(draft.doc));
  const [mode, setMode] = useState<"set" | "existing">(
    context.entry_count ? "existing" : "set",
  );
  const [starting, setStarting] = useState(() =>
    minorInput(context.opening_minor, account.currency),
  );
  const [date, setDate] = useState("");
  const problem = mappingError(mapping, draft.doc.headers.length);
  const startingMinor = parseStartingBalance(starting, account.currency);
  const preview = useMutation({
    mutationFn: () =>
      previewImport({
        account_id: account.id,
        filename: draft.filename,
        csv: draft.csv,
        source_group: draft.sourceGroup,
        delimiter: draft.doc.delimiter,
        mapping,
        starting_balance: {
          mode,
          amount_minor: mode === "set" ? startingMinor! : context.opening_minor,
          date,
        },
      }),
    onSuccess: onPreview,
  });
  const update = (
    key: keyof ImportMapping,
    value: string | number | undefined,
  ) => {
    setMapping({ ...mapping, [key]: value });
    preview.reset();
  };
  const column = (
    key:
      | "date"
      | "amount"
      | "debit"
      | "credit"
      | "note"
      | "category"
      | "currency"
      | "type"
      | "reference"
      | "exclude_report",
    label: string,
    required = false,
  ) => (
    <div key={key}>
      <label className="label" htmlFor={`map-${key}`}>
        {label}
        {!required ? " · optional" : ""}
      </label>
      <select
        id={`map-${key}`}
        className="input"
        value={mapping[key] ?? ""}
        onChange={(e) =>
          update(
            key,
            e.target.value === "" ? undefined : Number(e.target.value),
          )
        }
        required={required}
      >
        <option value="">
          {required ? "Choose a column" : "Skip this field"}
        </option>
        {draft.doc.headers.map((h, i) => (
          <option key={i} value={i}>
            {h} · column {i + 1}
          </option>
        ))}
      </select>
    </div>
  );
  return (
    <section className="import-panel" aria-labelledby="map-title">
      <p className="import-step">Step 2 of 3 · Match your columns</p>
      <h2 id="map-title" ref={heading} tabIndex={-1}>
        Make sure the numbers mean the right thing.
      </h2>
      <p className="muted">
        {draft.filename} · {draft.doc.row_count} records · {account.name} (
        {account.currency})
      </p>
      <details className="import-sample">
        <summary>See the first {draft.doc.samples.length} source rows</summary>
        <div
          className="import-table-scroll"
          tabIndex={0}
          role="region"
          aria-label="Source CSV sample"
        >
          <table>
            <thead>
              <tr>
                {draft.doc.headers.map((h, i) => (
                  <th key={i} scope="col">
                    {h} · {i + 1}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {draft.doc.samples.map((row, i) => (
                <tr key={i}>
                  {draft.doc.headers.map((_h, j) => (
                    <td key={j}>{row[j] || "—"}</td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </details>
      <form
        className="account-form"
        onSubmit={(e) => {
          e.preventDefault();
          if (!problem && (mode === "existing" || startingMinor !== null))
            preview.mutate();
        }}
      >
        <fieldset className="import-fields" disabled={preview.isPending}>
          <legend className="sr-only">Column mapping</legend>
          <div className="import-grid">
            {column("date", "Date", true)}
            <div>
              <label className="label" htmlFor="map-date-format">
                Date format
              </label>
              <select
                id="map-date-format"
                className="input"
                value={mapping.date_format}
                onChange={(e) => update("date_format", e.target.value)}
                required
              >
                <option value="" disabled>
                  Choose explicitly
                </option>
                <option value="iso">
                  Year-month-day · 2024-03-04 / ISO timestamp
                </option>
                <option value="day_first">
                  Day first · 04/03/2024 = 4 March
                </option>
                <option value="month_first">
                  Month first · 03/04/2024 = March 4
                </option>
              </select>
            </div>
            <div>
              <label className="label" htmlFor="map-amount-mode">
                Amount columns
              </label>
              <select
                id="map-amount-mode"
                className="input"
                value={mapping.amount_mode}
                onChange={(e) => update("amount_mode", e.target.value)}
              >
                <option value="signed">One signed amount column</option>
                <option value="debit_credit">
                  Separate debit / credit columns
                </option>
              </select>
            </div>
            <div>
              <label className="label" htmlFor="map-number-style">
                Number format in the file
              </label>
              <select
                id="map-number-style"
                className="input"
                value={mapping.number_style}
                onChange={(e) => update("number_style", e.target.value)}
              >
                <option value="dot">Decimal point · 1,234.56</option>
                <option value="comma">Decimal comma · 1.234,56</option>
              </select>
            </div>
            {mapping.amount_mode === "signed" ? (
              <>
                {column("amount", "Amount", true)}
                <div>
                  <label className="label" htmlFor="map-sign">
                    Positive amounts mean
                  </label>
                  <select
                    id="map-sign"
                    className="input"
                    value={mapping.sign_convention}
                    onChange={(e) => update("sign_convention", e.target.value)}
                  >
                    <option value="income_positive">Money in · income</option>
                    <option value="expense_positive">
                      Money out · expense
                    </option>
                  </select>
                </div>
              </>
            ) : (
              <>
                {column("debit", "Debit / money out")}
                {column("credit", "Credit / money in")}
              </>
            )}
            {column("note", "Description")}
            {column("category", "Category")}
          </div>
          <details>
            <summary>Currency, transaction type, and bank reference</summary>
            <div className="import-grid">
              {column("currency", "Currency")}
              {column("type", "Type")}
              {column("reference", "Unique bank reference")}
              {column("exclude_report", "Exclude from reports")}
            </div>
            <p className="input-hint">
              Expense types mean money out. Income and refunds must be positive
              after applying your sign rule. Currency must match{" "}
              {account.currency}. A bank reference helps detect repeated
              imports.
            </p>
          </details>
        </fieldset>
        <fieldset className="import-starting" disabled={preview.isPending}>
          <legend>Starting balance</legend>
          <p className="input-hint">
            Use the balance just before the activity you are importing. Today's
            bank balance would count this activity twice.
          </p>
          <label className="import-radio">
            <input
              type="radio"
              name="starting-mode"
              value="existing"
              checked={mode === "existing"}
              onChange={() => {
                setMode("existing");
                preview.reset();
              }}
            />
            <span>
              Keep existing starting balance ·{" "}
              {formatMoney(context.opening_minor, account.currency)}
              {context.tracking_start_at
                ? ` from ${context.tracking_start_at.slice(0, 10)} (UTC)`
                : ""}
            </span>
          </label>
          <label className="import-radio">
            <input
              type="radio"
              name="starting-mode"
              value="set"
              checked={mode === "set"}
              onChange={() => {
                setMode("set");
                preview.reset();
              }}
            />
            <span>
              {context.entry_count
                ? "Correct the starting balance or its date"
                : "Set a starting balance for this history"}
            </span>
          </label>
          {mode === "set" && (
            <div className="import-grid">
              <div>
                <label className="label" htmlFor="import-starting">
                  Starting balance · {account.currency}
                </label>
                <input
                  id="import-starting"
                  className="input"
                  value={starting}
                  onChange={(e) => {
                    setStarting(e.target.value);
                    preview.reset();
                  }}
                  inputMode="decimal"
                  required
                  aria-invalid={startingMinor === null}
                />
                <p
                  className={
                    startingMinor === null ? "field-error" : "input-hint"
                  }
                >
                  {startingMinor === null
                    ? "Enter an exact amount for this currency."
                    : "Zero and negative balances are allowed."}
                </p>
              </div>
              <div>
                <label className="label" htmlFor="import-start-date">
                  Start date · UTC, optional
                </label>
                <input
                  id="import-start-date"
                  className="input"
                  type="date"
                  max={new Date().toISOString().slice(0, 10)}
                  value={date}
                  onChange={(e) => {
                    setDate(e.target.value);
                    preview.reset();
                  }}
                />
                <p className="input-hint">
                  Blank uses the earliest valid date in this file. Any existing
                  starting entry is reversed before its replacement is saved.
                </p>
              </div>
            </div>
          )}
          <p className="input-hint">
            Current recorded balance:{" "}
            {formatMoney(context.balance_minor, account.currency)}. This stays
            unchanged until you confirm the import.
          </p>
        </fieldset>
        {problem && <p className="input-hint">{problem}</p>}
        {preview.isError && (
          <p role="alert" className="form-error">
            {apiError(preview.error)}
          </p>
        )}
        <div className="import-actions">
          <button
            className="btn"
            type="button"
            onClick={onBack}
            disabled={preview.isPending}
          >
            Back
          </button>
          <button
            className="btn btn-primary"
            disabled={
              !!problem ||
              (mode === "set" && startingMinor === null) ||
              preview.isPending
            }
          >
            {preview.isPending ? "Preparing review…" : "Preview import"}
            <Icon name="arrow" width="16" height="16" />
          </button>
        </div>
      </form>
    </section>
  );
}

function Review({
  view,
  preview,
  onEdit,
  onComplete,
}: {
  view: ImportView;
  preview: ImportPreview;
  onEdit: () => void;
  onComplete: (r: ImportReceipt) => void;
}) {
  const heading = useStepHeading();
  const qc = useQueryClient();
  const [selected, setSelected] = useState(() => readyRows(preview));
  const [approved, setApproved] = useState<number[]>([]);
  const [reviewed, setReviewed] = useState(false);
  const [page, setPage] = useState(0);
  const pageSize = 100;
  const visibleRows = preview.rows.slice(
    page * pageSize,
    (page + 1) * pageSize,
  );
  const projected = importProjection(preview, selected);
  const commit = useMutation({
    mutationFn: () => commitImport(view.id, selected, approved),
    onSuccess: (receipt) => {
      onComplete(receipt);
      void refreshLedger(qc);
    },
    onError: () => {
      void qc.invalidateQueries({ queryKey: ["import", view.id] });
    },
  });
  const toggle = (number: number, duplicate: boolean) => {
    setReviewed(false);
    commit.reset();
    setSelected((prev) =>
      prev.includes(number)
        ? prev.filter((n) => n !== number)
        : [...prev, number],
    );
    if (duplicate)
      setApproved((prev) =>
        prev.includes(number)
          ? prev.filter((n) => n !== number)
          : [...prev, number],
      );
  };
  const disabled = commit.isPending;
  const canCommit =
    reviewed &&
    projected &&
    !preview.plan_errors.length &&
    (selected.length > 0 || preview.starting_balance.mode === "existing");
  return (
    <section className="import-panel" aria-labelledby="review-import-title">
      <p className="import-step">Step 3 of 3 · Review</p>
      <h2 id="review-import-title" ref={heading} tabIndex={-1}>
        Choose what to bring in.
      </h2>
      <p className="muted">
        {view.filename} → {preview.context.name} · {preview.context.currency}.
        Nothing has been saved to your ledger.
      </p>
      <div className="import-counts">
        <span>
          <strong>{preview.ready_count}</strong> ready
        </span>
        <span>
          <strong>{preview.duplicate_count}</strong> possible duplicates
        </span>
        <span>
          <strong>{preview.invalid_count}</strong> need correction
        </span>
      </div>
      <p className="input-hint">
        Possible duplicates are skipped by default. Identical purchases can be
        real; include them individually after reviewing the match. Fix invalid
        rows in the source file and upload again.
      </p>
      {!!preview.plan_errors.length && (
        <div role="alert" className="form-error">
          {preview.plan_errors.map((e) => (
            <p key={e}>{e}</p>
          ))}
        </div>
      )}
      <div className="import-selection-actions">
        <button
          className="text-button"
          disabled={disabled}
          onClick={() => {
            setSelected(readyRows(preview));
            setApproved([]);
            setReviewed(false);
          }}
        >
          Select ready rows
        </button>
        <button
          className="text-button"
          disabled={disabled}
          onClick={() => {
            setSelected([]);
            setApproved([]);
            setReviewed(false);
          }}
        >
          Clear selection
        </button>
        <span>
          {selected.length} selected of {preview.rows.length}
        </span>
      </div>
      {!!preview.other_source_count && (
        <p className="input-hint">
          {preview.other_source_count.toLocaleString()} records belong to other
          source accounts/currencies. They remain in your original file; import
          those accounts separately.
        </p>
      )}
      {!!preview.planned_count && (
        <p className="input-hint">
          Future-dated entries will be saved as plans. They will not change
          balances or recorded spending until you confirm they happened.
        </p>
      )}
      {preview.rows.length > pageSize && (
        <nav className="import-actions" aria-label="Import review pages">
          <button
            className="btn"
            disabled={disabled || page === 0}
            onClick={() => setPage((p) => p - 1)}
          >
            Previous rows
          </button>
          <span role="status">
            Showing {page * pageSize + 1}–
            {Math.min((page + 1) * pageSize, preview.rows.length)} of{" "}
            {preview.rows.length.toLocaleString()} rows
          </span>
          <button
            className="btn"
            disabled={disabled || (page + 1) * pageSize >= preview.rows.length}
            onClick={() => setPage((p) => p + 1)}
          >
            Next rows
          </button>
        </nav>
      )}
      <ol className="import-rows" aria-label="Rows to import">
        {visibleRows.map((row) => (
          <li
            key={row.number}
            className={row.errors.length ? "import-row invalid" : "import-row"}
          >
            <div className="import-row-main">
              <label className="import-row-choice">
                <input
                  type="checkbox"
                  checked={selected.includes(row.number)}
                  disabled={disabled || !!row.errors.length}
                  onChange={() => toggle(row.number, !!row.duplicates.length)}
                />
                <span className="sr-only">
                  {row.duplicates.length
                    ? "Include possible duplicate"
                    : "Include"}{" "}
                  row {row.number}
                </span>
                <span className="import-row-number" aria-hidden="true">
                  Row {row.number}
                </span>
              </label>
              <div className="import-row-description">
                <strong>{row.note || "No description"}</strong>
                <span>
                  {row.errors.length && !row.occurred_at.startsWith("0001")
                    ? row.occurred_at.slice(0, 10)
                    : row.occurred_at.startsWith("0001")
                      ? "Invalid date"
                      : row.date_only
                        ? row.occurred_at.slice(0, 10)
                        : new Date(row.occurred_at).toLocaleString()}{" "}
                  ·{" "}
                  {row.entry_type === "refund"
                    ? "Refund"
                    : row.amount_minor < 0
                      ? "Expense"
                      : "Income"}
                  {row.category
                    ? ` · ${row.category}${row.new_category ? " (new category)" : ""}`
                    : " · Uncategorized"}
                </span>
                {row.planned && (
                  <span>Planned · awaiting your confirmation</span>
                )}
                {row.exclude_from_reports && (
                  <span>
                    Excluded from spending reports · still affects balance when
                    recorded
                  </span>
                )}
                {row.source_ref && <span>Reference: {row.source_ref}</span>}
              </div>
              <strong className="import-row-money">
                {formatMoney(row.amount_minor, preview.context.currency)}
              </strong>
            </div>
            {row.errors.length > 0 && (
              <ul className="field-error">
                {row.errors.map((e, i) => (
                  <li key={i}>{e}</li>
                ))}
              </ul>
            )}
            {row.duplicates.length > 0 && (
              <details className="import-duplicates">
                <summary>
                  Possible duplicate · review{" "}
                  {row.duplicates.length === 1 ? "match" : "matches"}
                </summary>
                <ul>
                  {row.duplicates.map((d, i) => (
                    <li key={i}>
                      {d.planned_id
                        ? `Existing planned activity #${d.planned_id} · ${d.occurred_at?.slice(0, 10)}`
                        : d.row_number
                          ? `Also appears at source row ${d.row_number}`
                          : `Recorded transaction #${d.transaction_id}${d.account_name ? ` · ${d.account_name}` : ""} · ${d.occurred_at?.slice(0, 10)} · ${formatMoney(d.amount_minor ?? 0, preview.context.currency)}${d.note ? ` · ${d.note}` : ""}`}
                      {d.match_type === "reference"
                        ? " · same bank reference"
                        : d.match_type === "date_amount"
                          ? " · same date and amount"
                          : ""}
                    </li>
                  ))}
                </ul>
              </details>
            )}
          </li>
        ))}
      </ol>
      <div className="import-summary">
        <h3>Your account after this import</h3>
        <dl className="review-details">
          <div>
            <dt>Current recorded balance</dt>
            <dd>
              {formatMoney(
                preview.context.balance_minor,
                preview.context.currency,
              )}
            </dd>
          </div>
          {preview.starting_balance.mode === "set" && (
            <div>
              <dt>Starting component · {preview.starting_balance.date} UTC</dt>
              <dd>
                {formatMoney(
                  preview.context.opening_minor,
                  preview.context.currency,
                )}{" "}
                →{" "}
                {formatMoney(
                  preview.starting_balance.amount_minor,
                  preview.context.currency,
                )}
              </dd>
            </div>
          )}
          <div>
            <dt>Selected activity</dt>
            <dd>
              {projected
                ? formatMoney(projected.delta, preview.context.currency)
                : "Amount exceeds the exact range"}
            </dd>
          </div>
          <div>
            <dt>Projected balance</dt>
            <dd>
              {projected
                ? formatMoney(projected.balance, preview.context.currency)
                : "Reduce the selection"}
            </dd>
          </div>
        </dl>
        <p className="input-hint">
          Only categories in selected rows will be created. Imported entries are
          saved together. If your account changes, you must refresh this
          preview.
        </p>
      </div>
      <label className="import-confirm">
        <input
          type="checkbox"
          checked={reviewed}
          disabled={disabled}
          onChange={(e) => setReviewed(e.target.checked)}
        />
        <span>
          I reviewed the selected rows, possible duplicates, and starting
          balance.
        </span>
      </label>
      {commit.isError && (
        <p role="alert" className="form-error">
          {apiError(commit.error)} Retrying the same selection is safe. If the
          account changed, edit and refresh the preview.
        </p>
      )}
      <div className="import-actions">
        <button className="btn" disabled={disabled} onClick={onEdit}>
          Edit or refresh preview
        </button>
        <button
          className="btn btn-primary"
          disabled={!canCommit || disabled}
          onClick={() => commit.mutate()}
        >
          {disabled
            ? "Saving import…"
            : selected.length
              ? `Import ${selected.length} ${selected.length === 1 ? "record" : "records"}`
              : "Finish review"}
        </button>
      </div>
      <p className="input-hint">
        Review expires {new Date(view.expires_at).toLocaleString()}. You can
        return to this review from Recent imports.
      </p>
    </section>
  );
}

function Receipt({
  receipt: r,
  account,
  onNew,
  onAnotherSource,
}: {
  receipt: ImportReceipt;
  account?: Account;
  onNew: () => void;
  onAnotherSource?: () => void;
}) {
  const heading = useStepHeading();
  const [checking, setChecking] = useState(false);
  return (
    <section
      className="import-panel import-receipt"
      aria-labelledby="import-receipt-title"
    >
      <span className="import-receipt-icon">
        <Icon name="check" />
      </span>
      <h2 id="import-receipt-title" ref={heading} tabIndex={-1}>
        {r.imported_count ? "Your activity is ready." : "Review completed."}
      </h2>
      <p className="muted">
        {r.filename} · {r.account_name} · Import #{r.id}
      </p>
      <div className="import-counts">
        <span>
          <strong>{r.imported_count}</strong> imported
        </span>
        <span>
          <strong>{r.skipped_count}</strong> skipped
        </span>
        <span>
          <strong>{r.new_categories}</strong> new categories
        </span>
      </div>
      <p className="input-hint">
        Skipped: {r.invalid_count} invalid rows, {r.duplicate_skipped} possible
        duplicates, and any other rows you left unchecked.
      </p>
      <dl className="review-details">
        <div>
          <dt>Balance before import</dt>
          <dd>{formatMoney(r.balance_before_minor, r.currency)}</dd>
        </div>
        {r.starting_before_minor !== r.starting_after_minor && (
          <div>
            <dt>Starting balance component</dt>
            <dd>
              {formatMoney(r.starting_before_minor, r.currency)} →{" "}
              {formatMoney(r.starting_after_minor, r.currency)}
            </dd>
          </div>
        )}
        <div>
          <dt>Balance after import</dt>
          <dd>{formatMoney(r.balance_after_minor, r.currency)}</dd>
        </div>
      </dl>
      <p>
        Check this against your bank or wallet's posted balance to spot anything
        missing.
      </p>
      {!!r.planned_count && (
        <p>
          {r.planned_count.toLocaleString()} future entries saved as plans.{" "}
          <Link className="text-link" to="/planned">
            Review planned activity
          </Link>
        </p>
      )}
      {!!r.excluded_report_count && (
        <p>
          {r.excluded_report_count.toLocaleString()} entries retain their
          Exclude Report setting.
        </p>
      )}
      {!!r.other_source_count && onAnotherSource && (
        <p>
          <button className="btn" onClick={onAnotherSource}>
            Import another account from this file
          </button>
        </p>
      )}
      <div className="import-actions">
        {account?.active && (
          <button className="btn btn-primary" onClick={() => setChecking(true)}>
            Check balance
          </button>
        )}
        <Link className="btn" to={`/activity?import_id=${r.id}`}>
          Review imported activity
          <Icon name="arrow" width="16" height="16" />
        </Link>
        <button className="text-button" onClick={onNew}>
          Import another file
        </button>
      </div>
      <p className="input-hint">
        Saved {new Date(r.completed_at).toLocaleString()}. This receipt is safe
        to reload.
      </p>
      {checking && account && (
        <BalanceCheckDialog
          account={account}
          onClose={() => setChecking(false)}
        />
      )}
    </section>
  );
}
