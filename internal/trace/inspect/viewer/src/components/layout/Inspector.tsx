import { formatTime } from "../../lib/format";
import { eventSeverity } from "../../lib/traceEvents";
import { cn } from "../../lib/utils";
import type { TraceDetail, TraceEvent } from "../../types/trace";
import { Button } from "../ui/button";
import { PayloadPanel } from "../inspector/PayloadPanels";
import type { PointerEvent as ReactPointerEvent } from "react";

type InspectorProps = {
  event?: TraceEvent | null;
  detail?: TraceDetail;
  sheetOpen: boolean;
  onCloseSheet: () => void;
  onResizeStart: (event: ReactPointerEvent<HTMLButtonElement>) => void;
};

export function Inspector({ event, detail, sheetOpen, onCloseSheet, onResizeStart }: InspectorProps) {
  return (
    <>
      <div
        className={cn(
          "fixed inset-0 z-40 hidden bg-black/60 transition-opacity max-[880px]:block",
          sheetOpen ? "pointer-events-auto opacity-100" : "pointer-events-none opacity-0",
        )}
        onClick={onCloseSheet}
      />
      <aside
        className={cn(
          "group/inspector sticky top-0 z-50 h-screen overflow-auto bg-zinc-950/95 p-4 transition-transform max-[1180px]:col-span-2 max-[1180px]:h-auto max-[880px]:fixed max-[880px]:inset-y-0 max-[880px]:right-0 max-[880px]:w-[min(24rem,92vw)] max-[880px]:shadow-2xl max-[880px]:shadow-black/60",
          sheetOpen ? "max-[880px]:translate-x-0" : "max-[880px]:translate-x-full",
        )}
      >
        <Button
          onPointerDown={onResizeStart}
          className="absolute bottom-0 left-0 top-0 hidden w-2 cursor-col-resize bg-cyan-300/0 transition hover:bg-cyan-300/25 group-hover/inspector:block max-[880px]:hidden"
          title="Resize inspector"
          aria-label="Resize inspector"
        />
        <div className="rounded-lg border border-white/10 bg-white/[0.035] p-4">
          <div className="flex items-center justify-between gap-3">
            <div className="text-xs font-semibold uppercase tracking-[0.2em] text-stone-500">Inspector</div>
            <Button
              onClick={onCloseSheet}
              className="hidden h-8 w-8 shrink-0 place-items-center rounded-md border border-white/10 bg-white/[0.04] text-sm text-stone-300 max-[880px]:grid"
              aria-label="Close inspector"
            >
              ×
            </Button>
          </div>
          <h3 className="mt-2 break-words text-lg font-semibold">{event?.type || "No event selected"}</h3>
          <p className="mt-1 text-xs text-stone-500">{detail?.summary?.id || "Select a session"} {event ? `· #${event.index}` : ""}</p>
        </div>
        {event ? (
          <div className="mt-3 space-y-3">
            <InspectorSummary event={event} />
            <PayloadPanel event={event} />
          </div>
        ) : (
          <div className="mt-3 rounded-lg border border-dashed border-white/10 p-8 text-center text-sm text-stone-500">Choose an event from the timeline.</div>
        )}
      </aside>
    </>
  );
}

function InspectorSummary({ event }: { event: TraceEvent }) {
  const rows = [
    ["Timestamp", formatTime(event.timestamp)],
    ["Type", event.type],
    ["Severity", eventSeverity(event)],
  ];
  if (event.tool_invocation?.name) rows.push(["Tool", event.tool_invocation.name]);
  if (event.tool_invocation?.phase) rows.push(["Tool phase", event.tool_invocation.phase]);
  if (event.model_request?.model) rows.push(["Model", event.model_request.model]);
  if (event.model_response?.requires_tool_calls !== undefined) rows.push(["Requires tools", String(event.model_response.requires_tool_calls)]);

  return (
    <section className="rounded-lg border border-white/10 bg-white/[0.035] p-4">
      <div className="space-y-3">
        {rows.map(([label, value]) => (
          <div key={label} className="flex justify-between gap-3 border-b border-white/5 pb-2 last:border-0 last:pb-0">
            <span className="text-xs text-stone-500">{label}</span>
            <span className="text-right text-xs font-medium text-stone-200">{value}</span>
          </div>
        ))}
      </div>
    </section>
  );
}
