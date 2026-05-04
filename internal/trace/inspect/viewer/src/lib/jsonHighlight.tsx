import type { ReactNode } from "react";
import type { TraceEvent } from "../types/trace";

export function prettyEvent(event: TraceEvent): string {
  return JSON.stringify(event, null, 2);
}

export function highlightJSON(value: unknown): ReactNode[] {
  const text = String(value ?? "");
  const pattern = /("(?:\\.|[^"\\])*"(?=\s*:)|"(?:\\.|[^"\\])*"|true|false|null|-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?|[{}\[\],:])/g;
  const parts = [];
  let cursor = 0;
  let match;

  while ((match = pattern.exec(text)) !== null) {
    if (match.index > cursor) {
      parts.push(text.slice(cursor, match.index));
    }
    const token = match[0];
    parts.push(
      <span key={`${match.index}:${token}`} className={jsonTokenClass(token, text.slice(match.index + token.length))}>
        {token}
      </span>,
    );
    cursor = match.index + token.length;
  }

  if (cursor < text.length) {
    parts.push(text.slice(cursor));
  }

  return parts;
}

function jsonTokenClass(token: string, after = ""): string {
  if (token.startsWith('"')) {
    return /^\s*:/.test(after) ? "text-sky-300" : "text-emerald-200";
  }
  if (/^-?\d/.test(token)) return "text-amber-200";
  if (token === "true" || token === "false") return "text-violet-200";
  if (token === "null") return "text-stone-500";
  if (token === ":" || token === "," || token === "{" || token === "}" || token === "[" || token === "]") return "text-stone-500";
  return "text-stone-300";
}
