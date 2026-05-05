import { RefreshCcw } from "lucide-react";
import { queryClient } from "../../app/queryClient";
import { Button } from "../ui/button";
import { StatusBadge } from "../ui/status-badge";
import { Toggle } from "../ui/toggle";
import type { TraceDetail } from "../../types/trace";

type HeaderProps = {
  detail?: TraceDetail;
  selectedId: string;
  activeView: "events" | "memory";
  autoRefresh: boolean;
  singleEventMode: boolean;
  onActiveView: (view: "events" | "memory") => void;
  onAutoRefresh: (checked: boolean) => void;
  onSingleEventMode: (checked: boolean) => void;
};

export function Header({
  detail,
  selectedId,
  activeView,
  autoRefresh,
  singleEventMode,
  onActiveView,
  onAutoRefresh,
  onSingleEventMode,
}: HeaderProps) {
  const summary = detail?.summary;

  return (
    <header className="border-b border-white/10 px-5 py-4">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <StatusBadge status={summary?.final_status} />
            <span className="rounded-full border border-violet-300/20 bg-violet-300/10 px-2.5 py-1 text-xs font-medium text-violet-200">{summary?.api_mode || "api unknown"}</span>
            <span className="rounded-full border border-white/10 bg-white/[0.04] px-2.5 py-1 text-xs text-stone-300">{summary?.agent_name || "agent unknown"}</span>
          </div>
          <h2 className="mt-3 truncate text-2xl font-semibold tracking-tight">{summary?.id || "Select a trace session"}</h2>
          <p className="mt-1 truncate text-sm text-stone-400">{summary?.model || "Model telemetry"} · {summary?.path || "No trace selected"}</p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <div className="flex rounded-lg border border-white/10 bg-white/[0.035] p-1">
            {(["events", "memory"] as const).map((view) => (
              <Button
                key={view}
                onClick={() => onActiveView(view)}
                className={`rounded-md px-3 py-1.5 text-sm font-semibold capitalize transition ${
                  activeView === view ? "bg-cyan-300/15 text-cyan-100" : "text-stone-400 hover:bg-white/[0.04] hover:text-stone-200"
                }`}
              >
                {view}
              </Button>
            ))}
          </div>
          <Toggle label="Auto refresh" checked={autoRefresh} onChange={onAutoRefresh} />
          <Toggle label="Single event" checked={singleEventMode} onChange={onSingleEventMode} />
          <Button
            onClick={() => {
              queryClient.invalidateQueries({ queryKey: ["trace-sessions"] });
              if (selectedId) queryClient.invalidateQueries({ queryKey: ["trace-session", selectedId] });
            }}
            className="grid h-10 w-10 place-items-center rounded-lg border border-cyan-300/30 bg-cyan-300/10 text-cyan-100 transition hover:bg-cyan-300/15"
            aria-label="Refresh"
            title="Refresh"
          >
            <RefreshCcw size={18} strokeWidth={2.2} />
          </Button>
        </div>
      </div>
    </header>
  );
}
