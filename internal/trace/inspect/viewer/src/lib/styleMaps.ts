import type { EventGroup } from "../constants/events";
import type { TraceEvent } from "../types/trace";
import { eventSeverity } from "./traceEvents";

type EventColor = EventGroup["color"];

export function groupClass(color: EventColor): string {
  const map = {
    cyan: "border-cyan-300/30 bg-cyan-300/10 text-cyan-100",
    green: "border-emerald-300/30 bg-emerald-300/10 text-emerald-100",
    blue: "border-blue-300/30 bg-blue-300/10 text-blue-100",
    violet: "border-violet-300/30 bg-violet-300/10 text-violet-100",
    amber: "border-amber-300/30 bg-amber-300/10 text-amber-100",
    slate: "border-slate-300/30 bg-slate-300/10 text-slate-100",
    red: "border-red-300/30 bg-red-300/10 text-red-100",
  };
  return map[color] || map.slate;
}

export function barClass(color: EventColor): string {
  const map = {
    cyan: "bg-cyan-300",
    green: "bg-emerald-300",
    blue: "bg-blue-300",
    violet: "bg-violet-300",
    amber: "bg-amber-300",
    slate: "bg-slate-300",
    red: "bg-red-300",
  };
  return map[color] || map.slate;
}

export function dotClass(event: TraceEvent): string {
  const severity = eventSeverity(event);
  if (severity === "failure") return "bg-red-300 shadow-[0_0_16px_rgba(252,165,165,0.65)]";
  if (severity === "warning") return "bg-amber-300 shadow-[0_0_16px_rgba(252,211,77,0.55)]";
  if (event.type?.includes("tool")) return "bg-emerald-300";
  if (event.type?.includes("model")) return "bg-cyan-300";
  if (event.type?.includes("memory")) return "bg-violet-300";
  return "bg-stone-400";
}
