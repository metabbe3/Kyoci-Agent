import { useMemo, useState } from "react";
import {
  Bar,
  BarChart,
  CartesianGrid,
  Legend,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { BarChart3, Table as TableIcon } from "lucide-react";
import { cn } from "@/lib/utils";

// CsvBlock parses a CSV string and renders it as either a table (default) or
// a chart (toggle). Used by the Markdown component when a fenced code block
// has language=csv. Charts are auto-detected: first non-numeric column
// becomes the x-axis label, every numeric column becomes a series.
//
// Kept deliberately simple: bar chart and line chart only. No scatter, pie,
// or axis-range config. If the data doesn't chart cleanly, the user can
// still see the raw table.

type Parsed = {
  headers: string[];
  rows: string[][];
  numericCols: number[]; // indices of columns treated as numeric series
  labelCol: number; // index of the x-axis label column (first non-numeric)
};

function parseCSV(input: string): Parsed | null {
  const lines = input.split(/\r?\n/).filter((l) => l.trim().length > 0);
  if (lines.length < 2) return null; // need header + at least 1 row

  const headers = splitCSVLine(lines[0]);
  const rows: string[][] = [];
  for (let i = 1; i < lines.length; i++) {
    rows.push(splitCSVLine(lines[i]));
  }

  // Classify columns: a column is numeric if >=70% of its non-empty cells
  // parse as float. Matches the heuristic used by the Go excel tool.
  const numericCols: number[] = [];
  const colIsNumeric = headers.map((_, idx) => {
    let nonEmpty = 0;
    let numeric = 0;
    for (const r of rows) {
      const v = (r[idx] ?? "").trim();
      if (!v) continue;
      nonEmpty++;
      if (!isNaN(Number(v))) numeric++;
    }
    if (nonEmpty === 0) return false;
    return numeric * 10 >= nonEmpty * 7;
  });
  colIsNumeric.forEach((b, idx) => {
    if (b) numericCols.push(idx);
  });

  // First non-numeric column becomes the label. If all columns are numeric,
  // use row index as the label (column -1 sentinel).
  let labelCol = -1;
  for (let i = 0; i < headers.length; i++) {
    if (!colIsNumeric[i]) {
      labelCol = i;
      break;
    }
  }

  return { headers, rows, numericCols, labelCol };
}

// splitCSVLine handles simple CSV with quoted fields. Not a full RFC 4180
// parser — good enough for tool-emitted CSVs (no embedded newlines).
function splitCSVLine(line: string): string[] {
  const out: string[] = [];
  let cur = "";
  let inQuote = false;
  for (let i = 0; i < line.length; i++) {
    const c = line[i];
    if (inQuote) {
      if (c === '"' && line[i + 1] === '"') {
        cur += '"';
        i++;
      } else if (c === '"') {
        inQuote = false;
      } else {
        cur += c;
      }
    } else {
      if (c === '"') inQuote = true;
      else if (c === ",") {
        out.push(cur);
        cur = "";
      } else {
        cur += c;
      }
    }
  }
  out.push(cur);
  return out.map((s) => s.trim());
}

export function CsvBlock({ code }: { code: string }) {
  const [view, setView] = useState<"table" | "bar" | "line">("table");
  const parsed = useMemo(() => parseCSV(code), [code]);

  if (!parsed) {
    // Too small to render as table or chart — fall back to plain <pre>.
    return (
      <pre className="overflow-x-auto rounded-md border border-neutral-200 bg-neutral-50 p-3 text-xs">
        <code>{code}</code>
      </pre>
    );
  }

  const canChart = parsed.numericCols.length > 0 && parsed.rows.length > 0;

  const chartData = useMemo(() => {
    if (!canChart) return [];
    return parsed.rows.map((r, idx) => {
      const entry: Record<string, number | string> = {};
      const label =
        parsed.labelCol === -1
          ? `#${idx + 1}`
          : r[parsed.labelCol] ?? `#${idx + 1}`;
      entry["__label"] = label;
      for (const colIdx of parsed.numericCols) {
        const v = Number((r[colIdx] ?? "").trim());
        entry[parsed.headers[colIdx]] = isNaN(v) ? 0 : v;
      }
      return entry;
    });
  }, [parsed, canChart]);

  return (
    <div className="my-2 rounded-md border border-neutral-200 bg-white">
      <div className="flex items-center justify-between border-b border-neutral-100 bg-neutral-50 px-3 py-1.5">
        <span className="text-xs font-medium text-neutral-600">
          {parsed.rows.length} row{parsed.rows.length === 1 ? "" : "s"} ·{" "}
          {parsed.headers.length} col
          {parsed.headers.length === 1 ? "" : "s"}
          {canChart && ` · ${parsed.numericCols.length} numeric`}
        </span>
        <div className="flex items-center gap-1">
          <ViewBtn
            active={view === "table"}
            onClick={() => setView("table")}
            icon={<TableIcon className="h-3 w-3" />}
            label="Table"
          />
          {canChart && (
            <ViewBtn
              active={view === "bar"}
              onClick={() => setView("bar")}
              icon={<BarChart3 className="h-3 w-3" />}
              label="Bar"
            />
          )}
          {canChart && (
            <ViewBtn
              active={view === "line"}
              onClick={() => setView("line")}
              icon={<LineChartIcon />}
              label="Line"
            />
          )}
        </div>
      </div>

      {view === "table" && (
        <div className="max-h-72 overflow-auto">
          <table className="w-full text-xs">
            <thead className="sticky top-0 bg-white">
              <tr>
                {parsed.headers.map((h, i) => (
                  <th
                    key={i}
                    className="border-b border-neutral-200 px-2 py-1 text-left font-medium text-neutral-700"
                  >
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {parsed.rows.map((r, ri) => (
                <tr key={ri} className="hover:bg-neutral-50">
                  {parsed.headers.map((_, ci) => (
                    <td
                      key={ci}
                      className="border-b border-neutral-100 px-2 py-1 text-neutral-800"
                    >
                      {r[ci] ?? ""}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {view !== "table" && canChart && (
        <div className="h-72 p-2">
          <ResponsiveContainer width="100%" height="100%">
            {view === "bar" ? (
              <BarChart data={chartData}>
                <CartesianGrid strokeDasharray="3 3" stroke="#e5e5e5" />
                <XAxis dataKey="__label" tick={{ fontSize: 11 }} />
                <YAxis tick={{ fontSize: 11 }} />
                <Tooltip />
                <Legend />
                {parsed.numericCols.map((idx) => (
                  <Bar
                    key={idx}
                    dataKey={parsed.headers[idx]}
                    fill={seriesColor(idx)}
                  />
                ))}
              </BarChart>
            ) : (
              <LineChart data={chartData}>
                <CartesianGrid strokeDasharray="3 3" stroke="#e5e5e5" />
                <XAxis dataKey="__label" tick={{ fontSize: 11 }} />
                <YAxis tick={{ fontSize: 11 }} />
                <Tooltip />
                <Legend />
                {parsed.numericCols.map((idx) => (
                  <Line
                    key={idx}
                    type="monotone"
                    dataKey={parsed.headers[idx]}
                    stroke={seriesColor(idx)}
                    strokeWidth={2}
                    dot={{ r: 3 }}
                  />
                ))}
              </LineChart>
            )}
          </ResponsiveContainer>
        </div>
      )}
    </div>
  );
}

function ViewBtn({
  active,
  onClick,
  icon,
  label,
}: {
  active: boolean;
  onClick: () => void;
  icon: React.ReactNode;
  label: string;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[11px] font-medium transition-colors",
        active
          ? "bg-neutral-900 text-white"
          : "text-neutral-600 hover:bg-neutral-200",
      )}
    >
      {icon}
      {label}
    </button>
  );
}

// tiny inline line-chart icon (lucide's LineChart icon is heavy for this use)
function LineChartIcon() {
  return (
    <svg
      width="12"
      height="12"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <path d="M3 17l6-6 4 4 8-8" />
    </svg>
  );
}

// seriesColor returns a stable color per numeric column index. Six-color
// palette; cycles for >6 series.
function seriesColor(idx: number): string {
  const palette = [
    "#2563eb", // blue-600
    "#16a34a", // green-600
    "#dc2626", // red-600
    "#9333ea", // purple-600
    "#ea580c", // orange-600
    "#0891b2", // cyan-600
  ];
  return palette[idx % palette.length];
}
