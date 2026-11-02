import { useId } from "react";
import type { ReportSelection } from "../lib/report-period";
import { defaultReportSelection } from "../lib/spending";

export default function PeriodPicker({
  value,
  onChange,
  showTimezone = true,
}: {
  value: ReportSelection;
  onChange: (selection: ReportSelection) => void;
  showTimezone?: boolean;
}) {
  const id = useId();
  return (
    <div className="period-picker">
      <div>
        <label className="label" htmlFor={`${id}-period`}>
          Period
        </label>
        <select
          id={`${id}-period`}
          className="input"
          value={value.type}
          onChange={(e) =>
            onChange(
              defaultReportSelection(e.target.value as ReportSelection["type"]),
            )
          }
        >
          <option value="weekly">Weekly</option>
          <option value="monthly">Monthly</option>
          <option value="yearly">Yearly</option>
          <option value="custom">Custom dates</option>
          <option value="all">All time</option>
        </select>
      </div>
      {value.type === "monthly" && (
        <div>
          <label className="label" htmlFor={`${id}-month`}>
            Month
          </label>
          <input
            id={`${id}-month`}
            className="input"
            type="month"
            value={value.month}
            onChange={(e) =>
              onChange({ type: "monthly", month: e.target.value })
            }
          />
        </div>
      )}
      {value.type === "weekly" && (
        <div>
          <label className="label" htmlFor={`${id}-week`}>
            Week containing
          </label>
          <input
            id={`${id}-week`}
            className="input"
            type="date"
            value={value.week_of}
            onChange={(e) =>
              onChange({ type: "weekly", week_of: e.target.value })
            }
          />
        </div>
      )}
      {value.type === "yearly" && (
        <div>
          <label className="label" htmlFor={`${id}-year`}>
            Year
          </label>
          <input
            id={`${id}-year`}
            className="input"
            type="number"
            min="1"
            max="9999"
            value={value.year}
            onChange={(e) => onChange({ type: "yearly", year: e.target.value })}
          />
        </div>
      )}
      {value.type === "custom" && (
        <>
          <div>
            <label className="label" htmlFor={`${id}-start`}>
              Start date
            </label>
            <input
              id={`${id}-start`}
              className="input"
              type="date"
              value={value.start_date}
              onChange={(e) =>
                onChange({ ...value, start_date: e.target.value })
              }
            />
          </div>
          <div>
            <label className="label" htmlFor={`${id}-end`}>
              End date
            </label>
            <input
              id={`${id}-end`}
              className="input"
              type="date"
              value={value.end_date}
              min={value.start_date}
              onChange={(e) => onChange({ ...value, end_date: e.target.value })}
            />
          </div>
        </>
      )}
      {showTimezone && (
        <span className="period-timezone">
          UTC calendar dates · end date included
          {value.type === "weekly" ? " · Monday–Sunday" : ""}
        </span>
      )}
    </div>
  );
}
