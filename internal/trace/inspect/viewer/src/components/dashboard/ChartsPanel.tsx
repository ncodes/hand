import { EVENT_GROUPS } from "../../constants/events";
import { compactNumber, formatTimeOnly } from "../../lib/format";
import { barClass } from "../../lib/styleMaps";
import type { TraceDetail, TraceMetrics } from "../../types/trace";
import { Button } from "../ui/button";
import { Card } from "../ui/card";
import type { Dispatch, SetStateAction } from "react";

const MAX_TOKEN_USAGE_POINTS = 12;

type ChartsPanelProps = {
  detail?: TraceDetail;
  metrics: TraceMetrics;
  expandedCharts: Set<string>;
  setExpandedCharts: Dispatch<SetStateAction<Set<string>>>;
};

export function ChartsPanel({ detail, metrics, expandedCharts, setExpandedCharts }: ChartsPanelProps) {
  const events = detail?.timeline ?? [];
  const distribution = EVENT_GROUPS.map((group) => ({
    ...group,
    count: events.filter((event) => group.types.includes(event.type)).length,
  }));
  const maxCount = Math.max(...distribution.map((group) => group.count), 1);
  const allTokenPoints = events
    .filter((event) => event.context_event?.total_tokens)
    .map((event) => ({
      index: event.index,
      time: formatTimeOnly(event.timestamp),
      total: event.context_event.total_tokens,
    }));
  const tokenPoints = allTokenPoints.slice(-MAX_TOKEN_USAGE_POINTS);
  const gauge = Math.min(100, Math.round((metrics.maxTokens / Math.max(metrics.contextLimit, 1)) * 100));

  function toggleChart(id: string) {
    const next = new Set(expandedCharts);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    setExpandedCharts(next);
  }

  return (
    <section className="grid grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)_minmax(0,0.8fr)] gap-3 max-[1280px]:grid-cols-1">
      <Card className="min-w-0 overflow-hidden p-4">
        <Button onClick={() => toggleChart("tokens")} className="mb-5 flex w-full items-start justify-between gap-3 text-left">
          <span className="min-w-0">
            <span className="block text-sm font-semibold">Context token usage</span>
            <span className="mt-1 block truncate text-xs text-stone-500">
              {allTokenPoints.length
                ? `${Math.min(allTokenPoints.length, MAX_TOKEN_USAGE_POINTS)} recent of ${allTokenPoints.length} usage records · peak ${compactNumber(metrics.maxTokens)}`
                : "No context usage records"}
            </span>
          </span>
          <span className="hidden shrink-0 text-xs text-stone-500 max-[880px]:block">{expandedCharts.has("tokens") ? "Hide" : "Show"}</span>
        </Button>
        <div className={`${expandedCharts.has("tokens") ? "max-[880px]:block" : "max-[880px]:hidden"}`}>
          {tokenPoints.length ? (
            <>
              <div className="flex h-40 min-w-0 items-end justify-center gap-3 overflow-hidden pt-2">
                {tokenPoints.map((point) => (
                  <div key={point.index} className="flex min-w-8 flex-none basis-8 flex-col items-center gap-2">
                    <div className="h-4 text-[0.65rem] font-medium leading-4 text-cyan-100">{compactNumber(point.total)}</div>
                    <div
                      className="w-full max-w-8 rounded-t bg-cyan-300/70 max-[880px]:max-w-3"
                      title={`Event #${point.index}: ${compactNumber(point.total)} tokens at ${point.time}`}
                      style={{ height: `${Math.max(6, (point.total / Math.max(metrics.maxTokens, 1)) * 104)}px` }}
                    />
                    <div className="font-mono text-[0.65rem] text-stone-500">#{point.index}</div>
                  </div>
                ))}
              </div>
            </>
          ) : (
            <div className="grid h-32 place-items-center rounded-md border border-dashed border-white/10 text-xs text-stone-500">
              No token usage records found.
            </div>
          )}
        </div>
      </Card>
      <Card className="p-4">
        <Button onClick={() => toggleChart("mix")} className="mb-3 flex w-full items-center justify-between gap-3 text-left">
          <h3 className="text-sm font-semibold">Event mix</h3>
          <span className="text-xs text-stone-500">{events.length} records</span>
          <span className="hidden text-xs text-stone-500 max-[880px]:block">{expandedCharts.has("mix") ? "Hide" : "Show"}</span>
        </Button>
        <div className={`${expandedCharts.has("mix") ? "max-[880px]:block" : "max-[880px]:hidden"} space-y-2`}>
          {distribution.map((group) => (
            <div key={group.id} className="grid grid-cols-[5.5rem_1fr_2rem] items-center gap-2 text-xs">
              <span className="text-stone-400">{group.label}</span>
              <div className="h-2 overflow-hidden rounded-full bg-white/10">
                <div className={`h-full ${barClass(group.color)}`} style={{ width: `${(group.count / maxCount) * 100}%` }} />
              </div>
              <span className="text-right text-stone-500">{group.count}</span>
            </div>
          ))}
        </div>
      </Card>
      <Card className="p-4">
        <Button onClick={() => toggleChart("budget")} className="flex w-full items-center justify-between gap-3 text-left">
          <span className="text-sm font-semibold">Context budget</span>
          <span className="hidden text-xs text-stone-500 max-[880px]:block">{expandedCharts.has("budget") ? "Hide" : "Show"}</span>
        </Button>
        <div className={`${expandedCharts.has("budget") ? "max-[880px]:grid" : "max-[880px]:hidden"} mt-5 grid place-items-center`}>
          <div className="grid h-28 w-28 place-items-center rounded-full border border-cyan-300/30 bg-[conic-gradient(rgb(34,211,238)_var(--value),rgba(255,255,255,0.08)_0)]" style={{ "--value": `${gauge}%` }}>
            <div className="grid h-20 w-20 place-items-center rounded-full bg-zinc-950">
              <span className="text-xl font-semibold">{gauge}%</span>
            </div>
          </div>
        </div>
        <div className={`${expandedCharts.has("budget") ? "max-[880px]:block" : "max-[880px]:hidden"} mt-4 text-center text-xs text-stone-500`}>{compactNumber(metrics.maxTokens)} / {compactNumber(metrics.contextLimit)} tokens</div>
      </Card>
    </section>
  );
}
