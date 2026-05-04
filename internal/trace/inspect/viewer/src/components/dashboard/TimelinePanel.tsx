import { useEffect, useRef } from "react";
import { formatTimeOnly } from "../../lib/format";
import { dotClass } from "../../lib/styleMaps";
import { eventPreview } from "../../lib/traceEvents";
import { cn } from "../../lib/utils";
import type { TraceEvent } from "../../types/trace";
import { Button } from "../ui/button";
import { Panel } from "../ui/card";

type TimelinePanelProps = {
  events: TraceEvent[];
  selectedEvent?: TraceEvent | null;
  singleEventMode: boolean;
  onSelect: (index: number | null) => void;
  loading: boolean;
};

export function TimelinePanel({ events, selectedEvent, singleEventMode, onSelect, loading }: TimelinePanelProps) {
  const panelRef = useRef(null);

  useEffect(() => {
    if (!selectedEvent) return;
    const row = panelRef.current?.querySelector(`[data-event-index="${selectedEvent.index}"]`);
    row?.scrollIntoView({ block: "nearest" });
  }, [selectedEvent]);

  return (
    <Panel className="min-w-0">
      <div className="flex items-center justify-between border-b border-white/10 px-4 py-3">
        <h3 className="text-sm font-semibold">Event timeline</h3>
        <span className="text-xs text-stone-500">{loading ? "Loading..." : `${events.length} visible`}</span>
      </div>
      <div ref={panelRef} className="max-h-[44rem] overflow-y-auto overflow-x-hidden p-3">
        {!events.length ? (
          <div className="rounded-lg border border-dashed border-white/10 p-8 text-center text-sm text-stone-500">No events match the current filters.</div>
        ) : (
          <div className="relative min-w-0 space-y-2 before:absolute before:left-[1.13rem] before:top-2 before:h-[calc(100%-1rem)] before:w-px before:bg-white/10 max-[520px]:before:left-[0.88rem]">
            {events.map((event) => {
              const active = selectedEvent?.index === event.index;
              return (
                <Button
                  key={event.index}
                  data-event-index={event.index}
                  onClick={() => onSelect(singleEventMode && active ? null : event.index)}
                  className={cn(
                    "relative grid w-full min-w-0 grid-cols-[2.25rem_7rem_minmax(0,1fr)_5rem] items-center gap-3 rounded-lg border p-3 text-left transition max-[520px]:grid-cols-[1.75rem_minmax(0,1fr)_2.5rem] max-[520px]:gap-2",
                    active ? "border-cyan-300/50 bg-cyan-300/10" : "border-white/10 bg-black/10 hover:border-white/20 hover:bg-white/[0.04]",
                  )}
                >
                  <span className={`relative z-10 h-3 w-3 rounded-full ${dotClass(event)}`} />
                  <span className="font-mono text-xs text-stone-500 max-[520px]:hidden">{formatTimeOnly(event.timestamp)}</span>
                  <span className="min-w-0">
                    <span className="hidden font-mono text-xs text-stone-500 max-[520px]:mb-1 max-[520px]:block">{formatTimeOnly(event.timestamp)}</span>
                    <span className="block truncate text-sm font-semibold text-stone-100">{event.type}</span>
                    <span className="block truncate text-xs text-stone-500">{eventPreview(event)}</span>
                  </span>
                  <span className="text-right text-xs text-stone-500">#{event.index}</span>
                </Button>
              );
            })}
          </div>
        )}
      </div>
    </Panel>
  );
}
