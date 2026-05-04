import { formatTime } from "../../lib/format";
import { cn } from "../../lib/utils";
import type { TraceSessionSummary } from "../../types/trace";
import { Button } from "../ui/button";
import { StatusBadge } from "../ui/status-badge";
import type { PointerEvent as ReactPointerEvent } from "react";

type SessionSidebarProps = {
  sessions: TraceSessionSummary[];
  loading: boolean;
  selectedId: string;
  collapsed: boolean;
  drawerOpen: boolean;
  onCloseDrawer: () => void;
  onToggleCollapsed: () => void;
  onResizeStart: (event: ReactPointerEvent<HTMLButtonElement>) => void;
  onSelect: (id: string) => void;
};

export function SessionSidebar({ sessions, loading, selectedId, collapsed, drawerOpen, onCloseDrawer, onToggleCollapsed, onResizeStart, onSelect }: SessionSidebarProps) {
  return (
    <>
      <div
        className={cn(
          "fixed inset-0 z-30 hidden bg-black/60 transition-opacity max-[880px]:block",
          drawerOpen ? "pointer-events-auto opacity-100" : "pointer-events-none opacity-0",
        )}
        onClick={onCloseDrawer}
      />
      <aside
        className={cn(
          "group/sidebar sticky top-0 z-40 h-screen overflow-visible border-r border-white/10 bg-zinc-950/95 transition-[transform,background-color,border-color] duration-150 ease-out max-[880px]:fixed max-[880px]:inset-y-0 max-[880px]:left-0 max-[880px]:w-[min(21rem,88vw)] max-[880px]:shadow-2xl max-[880px]:shadow-black/50",
          drawerOpen ? "max-[880px]:translate-x-0" : "max-[880px]:-translate-x-full",
        )}
      >
        <div className={cn(collapsed ? "px-2 py-4" : "p-4", "h-full overflow-auto transition-[padding] duration-150 ease-out max-[880px]:p-4")}>
          <div className={cn(collapsed ? "p-2" : "p-4", "mb-4 rounded-lg border border-white/10 bg-white/[0.03] transition-[padding] duration-150 ease-out")}>
            <div className="flex items-start justify-between gap-2">
              <div className={cn(collapsed ? "hidden max-[880px]:block" : "", "text-xs font-semibold uppercase tracking-[0.22em] text-cyan-300")}>Hand</div>
              <Button
                onClick={onToggleCollapsed}
                className="grid h-8 w-8 shrink-0 place-items-center rounded-md border border-white/10 bg-white/[0.04] text-sm text-stone-300 transition hover:border-cyan-300/40 hover:text-cyan-100 max-[880px]:hidden"
                title={collapsed ? "Expand sidebar" : "Collapse sidebar"}
                aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
              >
                {collapsed ? "»" : "«"}
              </Button>
              <Button
                onClick={onCloseDrawer}
                className="hidden h-8 w-8 shrink-0 place-items-center rounded-md border border-white/10 bg-white/[0.04] text-sm text-stone-300 max-[880px]:grid"
                aria-label="Close sessions drawer"
              >
                ×
              </Button>
            </div>
            {collapsed ? (
              <div className="mt-4 rotate-180 text-center text-xs font-semibold uppercase tracking-[0.18em] text-cyan-300 [writing-mode:vertical-rl] max-[880px]:hidden">Trace Viewer</div>
            ) : (
              <>
                <h1 className="mt-2 text-2xl font-semibold tracking-tight">Trace Viewer</h1>
                <p className="mt-2 text-sm text-stone-400">Session telemetry, model payloads, tool calls, memory events, and context pressure.</p>
              </>
            )}
            {collapsed && (
              <div className="hidden max-[880px]:block">
                <h1 className="mt-2 text-2xl font-semibold tracking-tight">Trace Viewer</h1>
                <p className="mt-2 text-sm text-stone-400">Session telemetry, model payloads, tool calls, memory events, and context pressure.</p>
              </div>
            )}
          </div>
          {(!collapsed || drawerOpen) && (
            <div className="mb-3 flex items-center justify-between text-xs text-stone-500">
              <span>{loading ? "Loading sessions" : `${sessions.length} sessions`}</span>
              <span>JSONL</span>
            </div>
          )}
          <div className="space-y-2">
            {sessions.map((session) => (
              <Button
                key={session.id}
                onClick={() => onSelect(session.id)}
                className={cn(
                  "w-full rounded-lg border text-left transition",
                  collapsed ? "grid min-h-12 place-items-center p-2 max-[880px]:block max-[880px]:p-3" : "p-3",
                  session.id === selectedId
                    ? "border-cyan-400/60 bg-cyan-400/10 shadow-[0_0_0_1px_rgba(34,211,238,0.16)]"
                    : "border-white/10 bg-white/[0.035] hover:border-white/20 hover:bg-white/[0.055]",
                )}
                title={session.id}
              >
                {collapsed && !drawerOpen ? (
                  <span className="h-2.5 w-2.5 rounded-full bg-emerald-300 shadow-[0_0_14px_rgba(110,231,183,0.55)]" />
                ) : (
                  <>
                    <div className="flex items-start justify-between gap-2">
                      <StatusBadge status={session.final_status} />
                      <span className="text-xs text-stone-500">{session.event_count || 0} events</span>
                    </div>
                    <div className="mt-3 truncate font-mono text-xs text-stone-300">{session.id}</div>
                    <div className="mt-2 text-sm font-medium text-stone-100">{session.model || session.agent_name || "Unknown model"}</div>
                    <div className="mt-1 text-xs text-stone-500">{formatTime(session.updated_at || session.started_at)}</div>
                  </>
                )}
              </Button>
            ))}
          </div>
        </div>
        {!collapsed && (
          <Button
            onPointerDown={onResizeStart}
            className="absolute bottom-0 right-[-5px] top-0 hidden w-2 cursor-col-resize bg-cyan-300/0 transition hover:bg-cyan-300/25 group-hover/sidebar:block max-[880px]:hidden"
            title="Resize sidebar"
            aria-label="Resize sidebar"
          />
        )}
      </aside>
    </>
  );
}
