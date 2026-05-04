import { compactNumber } from "../../lib/format";
import type { TraceMetrics } from "../../types/trace";
import { Card } from "../ui/card";

type MetricGridProps = {
  metrics: TraceMetrics;
};

export function MetricGrid({ metrics }: MetricGridProps) {
  const cards = [
    ["Events", metrics.events, "Total trace records"],
    ["Tool Calls", metrics.toolCalls, `${metrics.toolFailures} failed`],
    ["Model Requests", metrics.modelRequests, `${metrics.modelResponses} responses`],
    ["Context Tokens", compactNumber(metrics.maxTokens), "Peak observed"],
    ["Warnings", metrics.warnings, "Compaction/rules"],
    ["Duration", metrics.duration, "Session span"],
  ];

  return (
    <section className="grid grid-cols-6 gap-3 max-[1280px]:grid-cols-3 max-[880px]:grid-cols-2 max-[520px]:max-h-[22rem] max-[520px]:grid-cols-1 max-[520px]:overflow-y-auto max-[520px]:pr-1">
      {cards.map(([label, value, note]) => (
        <Card key={label} className="p-4 shadow-xl shadow-black/10 max-[520px]:p-5">
          <div className="text-xs font-medium uppercase tracking-[0.16em] text-stone-500">{label}</div>
          <div className="mt-2 text-2xl font-semibold text-white">{value}</div>
          <div className="mt-1 text-xs text-stone-500">{note}</div>
        </Card>
      ))}
    </section>
  );
}
