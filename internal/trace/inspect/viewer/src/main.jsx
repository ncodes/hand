import { QueryClient, QueryClientProvider, useQuery } from "@tanstack/react-query";
import { PanelLeftOpen, RefreshCcw, ScanSearch } from "lucide-react";
import React, { useEffect, useMemo, useState } from "react";
import { createRoot } from "react-dom/client";
import "./styles.css";

const POLL_INTERVAL_MS = 3000;

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 1000,
      refetchOnWindowFocus: false,
    },
  },
});

const EVENT_GROUPS = [
  { id: "model", label: "Model", color: "cyan", types: ["model.request", "model.response"] },
  { id: "tools", label: "Tools", color: "green", types: ["tool.invocation.started", "tool.invocation.completed"] },
  { id: "context", label: "Context", color: "blue", types: ["context.preflight", "context.postflight.usage_recorded", "context.compaction.triggered", "context.compaction.warning", "context.compaction.pending", "context.compaction.running", "context.compaction.succeeded", "context.compaction.failed"] },
  { id: "memory", label: "Memory", color: "violet", types: ["memory.search.started", "memory.retrieved", "memory.search.failed", "memory.extraction.started", "memory.extraction.window_loaded", "memory.extraction.candidates", "memory.extraction.memory_written", "memory.extraction.completed"] },
  { id: "summary", label: "Summary", color: "amber", types: ["summary.fallback.started", "context.summary.requested", "context.summary.saved", "context.summary.failed", "context.summary.parse_failed", "context.summary.applied"] },
  { id: "plan", label: "Plan", color: "slate", types: ["plan.updated", "plan.cleared", "plan.hydrated"] },
  { id: "errors", label: "Errors", color: "red", types: ["session.failed"] },
];

function App() {
  const [selectedId, setSelectedId] = useState("");
  const [selectedEventIndex, setSelectedEventIndex] = useState(null);
  const [autoRefresh, setAutoRefresh] = useLocalStorageBool("trace:auto-refresh", true);
  const [singleEventMode, setSingleEventMode] = useLocalStorageBool("trace:single-event", true);
  const [sidebarCollapsed, setSidebarCollapsed] = useLocalStorageBool("trace:sidebar-collapsed", false);
  const [sidebarWidth, setSidebarWidth] = useLocalStorageNumber("trace:sidebar-width", 304);
  const [inspectorWidth, setInspectorWidth] = useLocalStorageNumber("trace:inspector-width", 384);
  const [sidebarDrawerOpen, setSidebarDrawerOpen] = useState(false);
  const [inspectorSheetOpen, setInspectorSheetOpen] = useState(false);
  const [expandedCharts, setExpandedCharts] = useState(() => new Set());
  const effectiveSidebarWidth = clamp(sidebarWidth, 240, 520);
  const effectiveInspectorWidth = clamp(inspectorWidth, 320, 640);
  const [query, setQuery] = useState("");
  const [activeGroups, setActiveGroups] = useState(() => new Set(EVENT_GROUPS.map((group) => group.id)));
  const [severity, setSeverity] = useState("all");

  const sessionsQuery = useQuery({
    queryKey: ["trace-sessions"],
    queryFn: fetchSessions,
    refetchInterval: autoRefresh ? POLL_INTERVAL_MS : false,
  });

  const sessions = sessionsQuery.data?.sessions ?? [];

  useEffect(() => {
    if (!selectedId && sessions.length) {
      setSelectedId(sessions[0].id);
    }
  }, [selectedId, sessions]);

  const detailQuery = useQuery({
    queryKey: ["trace-session", selectedId],
    queryFn: () => fetchSession(selectedId),
    enabled: !!selectedId,
    refetchInterval: autoRefresh ? POLL_INTERVAL_MS : false,
  });

  const detail = detailQuery.data;
  const timeline = detail?.timeline ?? [];
  const filteredTimeline = useMemo(
    () => filterTimeline(timeline, activeGroups, severity, query),
    [timeline, activeGroups, severity, query],
  );
  const selectedEvent = useMemo(() => {
    if (!timeline.length) return null;
    const fallback = filteredTimeline[0] ?? timeline[0];
    if (selectedEventIndex === null) return fallback;
    return timeline.find((event) => event.index === selectedEventIndex) ?? fallback;
  }, [filteredTimeline, selectedEventIndex, timeline]);
  const metrics = useMemo(() => summarize(detail), [detail]);

  useEffect(() => {
    if (!selectedEvent && filteredTimeline.length) {
      setSelectedEventIndex(filteredTimeline[0].index);
    }
  }, [filteredTimeline, selectedEvent]);

  function startSidebarResize(event) {
    if (sidebarCollapsed) return;
    event.preventDefault();
    const startX = event.clientX;
    const startWidth = effectiveSidebarWidth;

    function handleMove(moveEvent) {
      setSidebarWidth(clamp(startWidth + moveEvent.clientX - startX, 240, 520));
    }

    function handleUp() {
      window.removeEventListener("pointermove", handleMove);
      window.removeEventListener("pointerup", handleUp);
      document.body.classList.remove("select-none", "cursor-col-resize");
    }

    document.body.classList.add("select-none", "cursor-col-resize");
    window.addEventListener("pointermove", handleMove);
    window.addEventListener("pointerup", handleUp);
  }

  function startInspectorResize(event) {
    event.preventDefault();
    const startX = event.clientX;
    const startWidth = effectiveInspectorWidth;

    function handleMove(moveEvent) {
      setInspectorWidth(clamp(startWidth + startX - moveEvent.clientX, 320, 640));
    }

    function handleUp() {
      window.removeEventListener("pointermove", handleMove);
      window.removeEventListener("pointerup", handleUp);
      document.body.classList.remove("select-none", "cursor-col-resize");
    }

    document.body.classList.add("select-none", "cursor-col-resize");
    window.addEventListener("pointermove", handleMove);
    window.addEventListener("pointerup", handleUp);
  }

  const layoutStyle = {
    "--trace-sidebar-width": sidebarCollapsed ? "4.5rem" : `${effectiveSidebarWidth}px`,
    "--trace-sidebar-width-narrow": sidebarCollapsed ? "4.5rem" : `${Math.min(effectiveSidebarWidth, 272)}px`,
    "--trace-inspector-width": `${effectiveInspectorWidth}px`,
  };

  return (
    <main className="h-screen min-h-0 overflow-hidden bg-stone-950 text-stone-100 max-[880px]:h-auto max-[880px]:min-h-screen max-[880px]:overflow-x-hidden max-[880px]:overflow-y-auto">
      <div
        className="grid h-screen min-h-0 [grid-template-columns:var(--trace-sidebar-width)_minmax(0,1fr)_var(--trace-inspector-width)] max-[1180px]:[grid-template-columns:var(--trace-sidebar-width-narrow)_minmax(0,1fr)] max-[880px]:block max-[880px]:h-auto max-[880px]:min-h-screen"
        style={layoutStyle}
      >
        <SessionSidebar
          sessions={sessions}
          loading={sessionsQuery.isLoading}
          selectedId={selectedId}
          collapsed={sidebarCollapsed}
          drawerOpen={sidebarDrawerOpen}
          onCloseDrawer={() => setSidebarDrawerOpen(false)}
          onToggleCollapsed={() => setSidebarCollapsed(!sidebarCollapsed)}
          onResizeStart={startSidebarResize}
          onSelect={(id) => {
            setSelectedId(id);
            setSelectedEventIndex(null);
            setSidebarDrawerOpen(false);
          }}
        />
        <section className="h-screen min-w-0 overflow-y-auto border-x border-white/10 bg-[radial-gradient(circle_at_top_left,rgba(20,184,166,0.12),transparent_34%),linear-gradient(180deg,rgba(255,255,255,0.045),transparent_18rem)] max-[880px]:h-auto max-[880px]:overflow-visible">
          <MobileTopBar
            onOpenSidebar={() => setSidebarDrawerOpen(true)}
            onOpenInspector={() => setInspectorSheetOpen(true)}
            hasEvent={!!selectedEvent}
          />
          <Header
            detail={detail}
            autoRefresh={autoRefresh}
            singleEventMode={singleEventMode}
            onAutoRefresh={setAutoRefresh}
            onSingleEventMode={setSingleEventMode}
            onRefresh={() => {
              queryClient.invalidateQueries({ queryKey: ["trace-sessions"] });
              if (selectedId) queryClient.invalidateQueries({ queryKey: ["trace-session", selectedId] });
            }}
          />
          <div className="space-y-5 p-5 max-[520px]:p-3">
            <MetricGrid metrics={metrics} />
            <FilterBar
              query={query}
              onQuery={setQuery}
              activeGroups={activeGroups}
              setActiveGroups={setActiveGroups}
              severity={severity}
              setSeverity={setSeverity}
            />
            <ChartsPanel
              detail={detail}
              metrics={metrics}
              expandedCharts={expandedCharts}
              setExpandedCharts={setExpandedCharts}
            />
            <TimelinePanel
              events={filteredTimeline}
              selectedEvent={selectedEvent}
              singleEventMode={singleEventMode}
              onSelect={(index) => {
                setSelectedEventIndex(index);
                if (index !== null && window.innerWidth <= 880) {
                  setInspectorSheetOpen(true);
                }
              }}
              loading={detailQuery.isLoading}
            />
          </div>
        </section>
        <Inspector
          event={selectedEvent}
          detail={detail}
          sheetOpen={inspectorSheetOpen}
          onCloseSheet={() => setInspectorSheetOpen(false)}
          onResizeStart={startInspectorResize}
        />
      </div>
    </main>
  );
}

function MobileTopBar({ onOpenSidebar, onOpenInspector, hasEvent }) {
  return (
    <div className="hidden grid-cols-[1fr_auto_1fr] items-center gap-2 border-b border-white/10 bg-zinc-950/90 px-3 py-3 backdrop-blur max-[880px]:grid">
      <button
        type="button"
        onClick={onOpenSidebar}
        className="grid h-10 w-10 place-items-center rounded-lg border border-cyan-300/30 bg-cyan-300/10 text-cyan-100"
        aria-label="Open sessions drawer"
        title="Sessions"
      >
        <PanelLeftOpen size={18} strokeWidth={2.2} />
      </button>
      <div className="justify-self-center text-center text-[0.68rem] font-semibold uppercase tracking-[0.2em] text-cyan-300">Trace Viewer</div>
      <button
        type="button"
        onClick={onOpenInspector}
        disabled={!hasEvent}
        className="grid h-10 w-10 place-items-center justify-self-end rounded-lg border border-white/10 bg-white/[0.04] text-stone-200 disabled:opacity-40"
        aria-label="Open inspector"
        title="Inspect"
      >
        <ScanSearch size={18} strokeWidth={2.2} />
      </button>
    </div>
  );
}

function SessionSidebar({ sessions, loading, selectedId, collapsed, drawerOpen, onCloseDrawer, onToggleCollapsed, onResizeStart, onSelect }) {
  return (
    <>
      <div
        className={`fixed inset-0 z-30 hidden bg-black/60 transition-opacity max-[880px]:block ${
          drawerOpen ? "pointer-events-auto opacity-100" : "pointer-events-none opacity-0"
        }`}
        onClick={onCloseDrawer}
      />
      <aside
        className={`group/sidebar sticky top-0 z-40 h-screen overflow-visible border-r border-white/10 bg-zinc-950/95 transition-transform max-[880px]:fixed max-[880px]:inset-y-0 max-[880px]:left-0 max-[880px]:w-[min(21rem,88vw)] max-[880px]:shadow-2xl max-[880px]:shadow-black/50 ${
          drawerOpen ? "max-[880px]:translate-x-0" : "max-[880px]:-translate-x-full"
        }`}
      >
        <div className={`${collapsed ? "px-2 py-4" : "p-4"} h-full overflow-auto max-[880px]:p-4`}>
        <div className={`${collapsed ? "p-2" : "p-4"} mb-4 rounded-lg border border-white/10 bg-white/[0.03]`}>
          <div className="flex items-start justify-between gap-2">
            <div className={`${collapsed ? "hidden max-[880px]:block" : ""} text-xs font-semibold uppercase tracking-[0.22em] text-cyan-300`}>Hand</div>
            <button
              type="button"
              onClick={onToggleCollapsed}
              className="grid h-8 w-8 shrink-0 place-items-center rounded-md border border-white/10 bg-white/[0.04] text-sm text-stone-300 transition hover:border-cyan-300/40 hover:text-cyan-100 max-[880px]:hidden"
              title={collapsed ? "Expand sidebar" : "Collapse sidebar"}
              aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
            >
              {collapsed ? "»" : "«"}
            </button>
            <button
              type="button"
              onClick={onCloseDrawer}
              className="hidden h-8 w-8 shrink-0 place-items-center rounded-md border border-white/10 bg-white/[0.04] text-sm text-stone-300 max-[880px]:grid"
              aria-label="Close sessions drawer"
            >
              ×
            </button>
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
            <button
              key={session.id}
              type="button"
              onClick={() => onSelect(session.id)}
              className={`w-full rounded-lg border text-left transition ${
                collapsed ? "grid min-h-12 place-items-center p-2 max-[880px]:block max-[880px]:p-3" : "p-3"
              } ${
                session.id === selectedId
                  ? "border-cyan-400/60 bg-cyan-400/10 shadow-[0_0_0_1px_rgba(34,211,238,0.16)]"
                  : "border-white/10 bg-white/[0.035] hover:border-white/20 hover:bg-white/[0.055]"
              }`}
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
            </button>
          ))}
        </div>
      </div>
      {!collapsed && (
        <button
          type="button"
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

function Header({ detail, autoRefresh, singleEventMode, onAutoRefresh, onSingleEventMode, onRefresh }) {
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
          <Toggle label="Auto refresh" checked={autoRefresh} onChange={onAutoRefresh} />
          <Toggle label="Single event" checked={singleEventMode} onChange={onSingleEventMode} />
          <button
            type="button"
            onClick={onRefresh}
            className="grid h-10 w-10 place-items-center rounded-lg border border-cyan-300/30 bg-cyan-300/10 text-cyan-100 transition hover:bg-cyan-300/15"
            aria-label="Refresh"
            title="Refresh"
          >
            <RefreshCcw size={18} strokeWidth={2.2} />
          </button>
        </div>
      </div>
    </header>
  );
}

function MetricGrid({ metrics }) {
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
        <article key={label} className="rounded-lg border border-white/10 bg-zinc-900/70 p-4 shadow-xl shadow-black/10 max-[520px]:p-5">
          <div className="text-xs font-medium uppercase tracking-[0.16em] text-stone-500">{label}</div>
          <div className="mt-2 text-2xl font-semibold text-white">{value}</div>
          <div className="mt-1 text-xs text-stone-500">{note}</div>
        </article>
      ))}
    </section>
  );
}

function FilterBar({ query, onQuery, activeGroups, setActiveGroups, severity, setSeverity }) {
  return (
    <section className="rounded-lg border border-white/10 bg-zinc-900/70 p-3">
      <div className="flex flex-wrap items-center gap-2 max-[520px]:block max-[520px]:space-y-2">
        <input
          value={query}
          onChange={(event) => onQuery(event.target.value)}
          placeholder="Search type, payload, tool name, model output..."
          className="min-w-0 flex-1 rounded-lg border border-white/10 bg-black/20 px-3 py-2 text-sm text-stone-100 outline-none placeholder:text-stone-600 focus:border-cyan-300/50 max-[520px]:w-full"
        />
        <select value={severity} onChange={(event) => setSeverity(event.target.value)} className="rounded-lg border border-white/10 bg-black/20 px-3 py-2 text-sm text-stone-200 outline-none max-[520px]:w-full">
          <option value="all">All severity</option>
          <option value="warning">Warnings</option>
          <option value="failure">Failures</option>
        </select>
      </div>
      <div className="mt-3 flex flex-wrap gap-2 max-[520px]:gap-1.5">
        {EVENT_GROUPS.map((group) => {
          const active = activeGroups.has(group.id);
          return (
            <button
              key={group.id}
              type="button"
              onClick={() => {
                const next = new Set(activeGroups);
                if (active) next.delete(group.id);
                else next.add(group.id);
                setActiveGroups(next);
              }}
              className={`rounded-full border px-3 py-1.5 text-xs font-semibold transition max-[520px]:px-2.5 ${active ? groupClass(group.color) : "border-white/10 bg-white/[0.03] text-stone-500"}`}
            >
              {group.label}
            </button>
          );
        })}
      </div>
    </section>
  );
}

function ChartsPanel({ detail, metrics, expandedCharts, setExpandedCharts }) {
  const events = detail?.timeline ?? [];
  const distribution = EVENT_GROUPS.map((group) => ({
    ...group,
    count: events.filter((event) => group.types.includes(event.type)).length,
  }));
  const maxCount = Math.max(...distribution.map((group) => group.count), 1);
  const tokenPoints = events
    .filter((event) => event.context_event?.total_tokens)
    .map((event) => ({
      index: event.index,
      time: formatTimeOnly(event.timestamp),
      total: event.context_event.total_tokens,
    }));
  const gauge = Math.min(100, Math.round((metrics.maxTokens / Math.max(metrics.contextLimit, 1)) * 100));

  function toggleChart(id) {
    const next = new Set(expandedCharts);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    setExpandedCharts(next);
  }

  return (
    <section className="grid grid-cols-[1.4fr_1fr_0.8fr] gap-3 max-[1280px]:grid-cols-1">
      <article className="rounded-lg border border-white/10 bg-zinc-900/70 p-4">
        <button
          type="button"
          onClick={() => toggleChart("tokens")}
          className="mb-5 flex w-full items-start justify-between gap-3 text-left"
        >
          <span className="min-w-0">
            <span className="block text-sm font-semibold">Context token usage</span>
            <span className="mt-1 block truncate text-xs text-stone-500">
              {tokenPoints.length ? `${tokenPoints.length} usage records · peak ${compactNumber(metrics.maxTokens)}` : "No context usage records"}
            </span>
          </span>
          <span className="hidden shrink-0 text-xs text-stone-500 max-[880px]:block">{expandedCharts.has("tokens") ? "Hide" : "Show"}</span>
        </button>
        <div className={`${expandedCharts.has("tokens") ? "max-[880px]:block" : "max-[880px]:hidden"}`}>
          {tokenPoints.length ? (
            <>
              <div className="flex h-36 items-end justify-center gap-4 pt-4">
                {tokenPoints.map((point) => (
                  <div key={point.index} className="flex min-w-10 flex-col items-center gap-2">
                    <div className="text-[0.65rem] font-medium text-cyan-100">{compactNumber(point.total)}</div>
                    <div
                      className="w-8 max-w-10 rounded-t bg-cyan-300/70 max-[880px]:w-3"
                      title={`Event #${point.index}: ${compactNumber(point.total)} tokens at ${point.time}`}
                      style={{ height: `${Math.max(6, (point.total / Math.max(metrics.maxTokens, 1)) * 128)}px` }}
                    />
                    <div className="font-mono text-[0.65rem] text-stone-500">#{point.index}</div>
                  </div>
                ))}
              </div>
              {tokenPoints.length < 3 && (
                <div className="mt-3 rounded-md border border-white/10 bg-black/20 px-3 py-2 text-xs text-stone-500">
                  Only {tokenPoints.length} usage records in this trace, so trend detail is limited.
                </div>
              )}
            </>
          ) : (
            <div className="grid h-32 place-items-center rounded-md border border-dashed border-white/10 text-xs text-stone-500">
              No token usage records found.
            </div>
          )}
        </div>
      </article>
      <article className="rounded-lg border border-white/10 bg-zinc-900/70 p-4">
        <button
          type="button"
          onClick={() => toggleChart("mix")}
          className="mb-3 flex w-full items-center justify-between gap-3 text-left"
        >
          <h3 className="text-sm font-semibold">Event mix</h3>
          <span className="text-xs text-stone-500">{events.length} records</span>
          <span className="hidden text-xs text-stone-500 max-[880px]:block">{expandedCharts.has("mix") ? "Hide" : "Show"}</span>
        </button>
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
      </article>
      <article className="rounded-lg border border-white/10 bg-zinc-900/70 p-4">
        <button
          type="button"
          onClick={() => toggleChart("budget")}
          className="flex w-full items-center justify-between gap-3 text-left"
        >
          <span className="text-sm font-semibold">Context budget</span>
          <span className="hidden text-xs text-stone-500 max-[880px]:block">{expandedCharts.has("budget") ? "Hide" : "Show"}</span>
        </button>
        <div className={`${expandedCharts.has("budget") ? "max-[880px]:grid" : "max-[880px]:hidden"} mt-5 grid place-items-center`}>
          <div className="grid h-28 w-28 place-items-center rounded-full border border-cyan-300/30 bg-[conic-gradient(rgb(34,211,238)_var(--value),rgba(255,255,255,0.08)_0)]" style={{ "--value": `${gauge}%` }}>
            <div className="grid h-20 w-20 place-items-center rounded-full bg-zinc-950">
              <span className="text-xl font-semibold">{gauge}%</span>
            </div>
          </div>
        </div>
        <div className={`${expandedCharts.has("budget") ? "max-[880px]:block" : "max-[880px]:hidden"} mt-4 text-center text-xs text-stone-500`}>{compactNumber(metrics.maxTokens)} / {compactNumber(metrics.contextLimit)} tokens</div>
      </article>
    </section>
  );
}

function TimelinePanel({ events, selectedEvent, singleEventMode, onSelect, loading }) {
  return (
    <section className="min-w-0 rounded-lg border border-white/10 bg-zinc-900/70">
      <div className="flex items-center justify-between border-b border-white/10 px-4 py-3">
        <h3 className="text-sm font-semibold">Event timeline</h3>
        <span className="text-xs text-stone-500">{loading ? "Loading..." : `${events.length} visible`}</span>
      </div>
      <div className="max-h-[44rem] overflow-y-auto overflow-x-hidden p-3">
        {!events.length ? (
          <div className="rounded-lg border border-dashed border-white/10 p-8 text-center text-sm text-stone-500">No events match the current filters.</div>
        ) : (
          <div className="relative min-w-0 space-y-2 before:absolute before:left-[1.13rem] before:top-2 before:h-[calc(100%-1rem)] before:w-px before:bg-white/10 max-[520px]:before:left-[0.88rem]">
            {events.map((event) => {
              const active = selectedEvent?.index === event.index;
              return (
                <button
                  key={event.index}
                  type="button"
                  onClick={() => onSelect(singleEventMode && active ? null : event.index)}
                  className={`relative grid w-full min-w-0 grid-cols-[2.25rem_7rem_minmax(0,1fr)_5rem] items-center gap-3 rounded-lg border p-3 text-left transition max-[520px]:grid-cols-[1.75rem_minmax(0,1fr)_2.5rem] max-[520px]:gap-2 ${
                    active ? "border-cyan-300/50 bg-cyan-300/10" : "border-white/10 bg-black/10 hover:border-white/20 hover:bg-white/[0.04]"
                  }`}
                >
                  <span className={`relative z-10 h-3 w-3 rounded-full ${dotClass(event)}`} />
                  <span className="font-mono text-xs text-stone-500 max-[520px]:hidden">{formatTimeOnly(event.timestamp)}</span>
                  <span className="min-w-0">
                    <span className="hidden font-mono text-xs text-stone-500 max-[520px]:mb-1 max-[520px]:block">{formatTimeOnly(event.timestamp)}</span>
                    <span className="block truncate text-sm font-semibold text-stone-100">{event.type}</span>
                    <span className="block truncate text-xs text-stone-500">{eventPreview(event)}</span>
                  </span>
                  <span className="text-right text-xs text-stone-500">#{event.index}</span>
                </button>
              );
            })}
          </div>
        )}
      </div>
    </section>
  );
}

function Inspector({ event, detail, sheetOpen, onCloseSheet, onResizeStart }) {
  return (
    <>
      <div
        className={`fixed inset-0 z-40 hidden bg-black/60 transition-opacity max-[880px]:block ${
          sheetOpen ? "pointer-events-auto opacity-100" : "pointer-events-none opacity-0"
        }`}
        onClick={onCloseSheet}
      />
      <aside
        className={`group/inspector sticky top-0 z-50 h-screen overflow-auto bg-zinc-950/95 p-4 transition-transform max-[1180px]:col-span-2 max-[1180px]:h-auto max-[880px]:fixed max-[880px]:inset-y-0 max-[880px]:right-0 max-[880px]:w-[min(24rem,92vw)] max-[880px]:shadow-2xl max-[880px]:shadow-black/60 ${
          sheetOpen ? "max-[880px]:translate-x-0" : "max-[880px]:translate-x-full"
        }`}
      >
      <button
        type="button"
        onPointerDown={onResizeStart}
        className="absolute bottom-0 left-0 top-0 hidden w-2 cursor-col-resize bg-cyan-300/0 transition hover:bg-cyan-300/25 group-hover/inspector:block max-[880px]:hidden"
        title="Resize inspector"
        aria-label="Resize inspector"
      />
      <div className="rounded-lg border border-white/10 bg-white/[0.035] p-4">
        <div className="flex items-center justify-between gap-3">
          <div className="text-xs font-semibold uppercase tracking-[0.2em] text-stone-500">Inspector</div>
          <button
            type="button"
            onClick={onCloseSheet}
            className="hidden h-8 w-8 shrink-0 place-items-center rounded-md border border-white/10 bg-white/[0.04] text-sm text-stone-300 max-[880px]:grid"
            aria-label="Close inspector"
          >
            ×
          </button>
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

function InspectorSummary({ event }) {
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

function PayloadPanel({ event }) {
  return (
    <section className="rounded-lg border border-white/10 bg-white/[0.035]">
      <div className="border-b border-white/10 px-4 py-3 text-sm font-semibold">Structured payload</div>
      <div className="space-y-3 p-4">
        {event.tool_invocation && <ToolPayload tool={event.tool_invocation} />}
        {event.model_request && <ModelRequestPayload request={event.model_request} />}
        {event.context_event && <KeyValuePayload payload={event.context_event} />}
        {event.plan_event && <KeyValuePayload payload={event.plan_event.summary} title="Plan summary" />}
        {event.workspace_rules && <KeyValuePayload payload={event.workspace_rules} title="Workspace rules" />}
        <pre className="max-h-96 overflow-auto rounded-lg border border-white/10 bg-black/40 p-3 font-mono text-xs leading-relaxed text-stone-300">
          <code>{highlightJSON(prettyEvent(event))}</code>
        </pre>
      </div>
    </section>
  );
}

function ToolPayload({ tool }) {
  return (
    <div className="rounded-lg border border-emerald-300/20 bg-emerald-300/5 p-3">
      <div className="flex flex-wrap gap-2 text-xs">
        <span className="rounded-full bg-emerald-300/10 px-2 py-1 text-emerald-200">Name: {tool.name || "unknown"}</span>
        <span className="rounded-full bg-emerald-300/10 px-2 py-1 text-emerald-200">Phase: {tool.phase || "unknown"}</span>
        {tool.pair_index !== null && tool.pair_index !== undefined && <span className="rounded-full bg-emerald-300/10 px-2 py-1 text-emerald-200">Pair: #{tool.pair_index}</span>}
      </div>
      {tool.input && <pre className="mt-3 max-h-40 overflow-auto rounded bg-black/35 p-2 font-mono text-xs text-stone-300">{tool.input}</pre>}
      {tool.content && <pre className="mt-3 max-h-40 overflow-auto rounded bg-black/35 p-2 font-mono text-xs text-stone-300">{tool.content}</pre>}
    </div>
  );
}

function ModelRequestPayload({ request }) {
  const metrics = request.context ?? {};
  return (
    <div className="grid grid-cols-2 gap-2">
      <MiniMetric label="Instruction chars" value={metrics.instruction_chars || 0} />
      <MiniMetric label="Messages" value={metrics.message_count || 0} />
      <MiniMetric label="Message chars" value={metrics.message_chars || 0} />
      <MiniMetric label="Tools" value={metrics.tool_count || 0} />
    </div>
  );
}

function KeyValuePayload({ payload, title }) {
  return (
    <div className="rounded-lg border border-white/10 bg-black/20 p-3">
      {title && <div className="mb-2 text-xs font-semibold uppercase tracking-[0.16em] text-stone-500">{title}</div>}
      <div className="space-y-1">
        {Object.entries(payload || {}).map(([key, value]) => (
          <div key={key} className="flex justify-between gap-3 text-xs">
            <span className="text-stone-500">{key}</span>
            <span className="text-right text-stone-300">{String(value)}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

function MiniMetric({ label, value }) {
  return (
    <div className="rounded-lg border border-white/10 bg-black/20 p-3">
      <div className="text-xs text-stone-500">{label}</div>
      <div className="mt-1 text-lg font-semibold">{compactNumber(value)}</div>
    </div>
  );
}

function Toggle({ label, checked, onChange }) {
  return (
    <label className="flex cursor-pointer items-center gap-2 rounded-lg border border-white/10 bg-white/[0.035] px-3 py-2 text-sm text-stone-300">
      <input type="checkbox" checked={checked} onChange={(event) => onChange(event.target.checked)} className="accent-cyan-300" />
      {label}
    </label>
  );
}

function StatusBadge({ status }) {
  const normalized = status || "incomplete";
  const cls = normalized.includes("fail")
    ? "border-red-300/30 bg-red-300/10 text-red-200"
    : normalized.includes("complete")
      ? "border-emerald-300/30 bg-emerald-300/10 text-emerald-200"
      : "border-amber-300/30 bg-amber-300/10 text-amber-200";
  return <span className={`rounded-full border px-2.5 py-1 text-xs font-semibold capitalize ${cls}`}>{normalized.replaceAll("_", " ")}</span>;
}

function useLocalStorageBool(key, initialValue) {
  const [value, setValue] = useState(() => {
    const stored = window.localStorage.getItem(key);
    if (stored === null) return initialValue;
    return stored === "true";
  });

  useEffect(() => {
    window.localStorage.setItem(key, String(value));
  }, [key, value]);

  return [value, setValue];
}

function useLocalStorageNumber(key, initialValue) {
  const [value, setValue] = useState(() => {
    const stored = window.localStorage.getItem(key);
    const parsed = Number(stored);
    return Number.isFinite(parsed) ? parsed : initialValue;
  });

  useEffect(() => {
    window.localStorage.setItem(key, String(value));
  }, [key, value]);

  return [value, setValue];
}

async function fetchSessions() {
  const response = await fetch("/api/sessions");
  if (!response.ok) throw new Error("failed to fetch trace sessions");
  return response.json();
}

async function fetchSession(id) {
  const response = await fetch(`/api/sessions/${encodeURIComponent(id)}`);
  if (!response.ok) throw new Error("failed to fetch trace session");
  return response.json();
}

function summarize(detail) {
  const events = detail?.timeline ?? [];
  const contextEvents = events.map((event) => event.context_event).filter(Boolean);
  const timestamps = events.map((event) => new Date(event.timestamp).getTime()).filter((value) => !Number.isNaN(value));
  const maxTokens = Math.max(0, ...contextEvents.map((event) => event.total_tokens || 0));
  const contextLimit = Math.max(1, ...contextEvents.map((event) => event.context_limit || 0));

  return {
    events: events.length,
    toolCalls: events.filter((event) => event.tool_invocation).length,
    toolFailures: events.filter((event) => event.tool_invocation?.phase === "completed" && failureText(event)).length,
    modelRequests: events.filter((event) => event.model_request).length,
    modelResponses: events.filter((event) => event.model_response).length,
    warnings: events.filter((event) => eventSeverity(event) !== "info").length,
    maxTokens,
    contextLimit,
    duration: timestamps.length > 1 ? formatDuration(Math.max(...timestamps) - Math.min(...timestamps)) : "n/a",
  };
}

function filterTimeline(events, activeGroups, severity, query) {
  const lowered = query.trim().toLowerCase();
  return events.filter((event) => {
    const group = EVENT_GROUPS.find((candidate) => candidate.types.includes(event.type));
    if (group && !activeGroups.has(group.id)) return false;
    if (severity === "warning" && eventSeverity(event) === "info") return false;
    if (severity === "failure" && eventSeverity(event) !== "failure") return false;
    if (!lowered) return true;
    return JSON.stringify(event).toLowerCase().includes(lowered);
  });
}

function eventSeverity(event) {
  if (!event) return "info";
  if (event.type?.includes("failed") || event.failure || failureText(event)) return "failure";
  if (event.type?.includes("warning") || event.workspace_rules) return "warning";
  return "info";
}

function failureText(event) {
  return event.tool_invocation?.content?.toLowerCase().includes("error") || event.tool_invocation?.content?.toLowerCase().includes("failed");
}

function eventPreview(event) {
  if (event.user_message?.message) return event.user_message.message;
  if (event.final_response?.message) return event.final_response.message;
  if (event.tool_invocation) return `${event.tool_invocation.name || "tool"} ${event.tool_invocation.phase || ""}`;
  if (event.model_request) return `${event.model_request.model || "model"} · ${event.model_request.context?.message_count || 0} messages`;
  if (event.model_response) return event.model_response.output_text || `${event.model_response.tool_calls?.length || 0} tool calls`;
  if (event.context_event) return `${compactNumber(event.context_event.total_tokens || 0)} tokens`;
  if (event.summary_event) return event.summary_event.error || `source offset ${event.summary_event.source_end_offset || 0}`;
  if (event.plan_event) return `${event.plan_event.summary?.completed || 0}/${event.plan_event.summary?.total || 0} steps complete`;
  if (event.workspace_rules) return `truncated to ${compactNumber(event.workspace_rules.truncated_length || 0)} chars`;
  if (event.generic_payload_raw) return event.generic_payload_raw;
  return "Trace event";
}

function prettyEvent(event) {
  return JSON.stringify(event, null, 2);
}

function highlightJSON(value) {
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

function jsonTokenClass(token, after = "") {
  if (token.startsWith('"')) {
    return /^\s*:/.test(after) ? "text-sky-300" : "text-emerald-200";
  }
  if (/^-?\d/.test(token)) return "text-amber-200";
  if (token === "true" || token === "false") return "text-violet-200";
  if (token === "null") return "text-stone-500";
  if (token === ":" || token === "," || token === "{" || token === "}" || token === "[" || token === "]") return "text-stone-500";
  return "text-stone-300";
}

function formatTime(value) {
  if (!value) return "Unknown";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return String(value);
  return date.toLocaleString();
}

function formatTimeOnly(value) {
  if (!value) return "--:--:--";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return String(value);
  return date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

function formatDuration(ms) {
  if (!Number.isFinite(ms) || ms < 0) return "n/a";
  if (ms < 1000) return `${ms}ms`;
  const seconds = Math.round(ms / 1000);
  if (seconds < 60) return `${seconds}s`;
  return `${Math.floor(seconds / 60)}m ${seconds % 60}s`;
}

function compactNumber(value) {
  return new Intl.NumberFormat("en", { notation: "compact", maximumFractionDigits: 1 }).format(Number(value || 0));
}

function clamp(value, min, max) {
  return Math.min(max, Math.max(min, value));
}

function groupClass(color) {
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

function barClass(color) {
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

function dotClass(event) {
  const severity = eventSeverity(event);
  if (severity === "failure") return "bg-red-300 shadow-[0_0_16px_rgba(252,165,165,0.65)]";
  if (severity === "warning") return "bg-amber-300 shadow-[0_0_16px_rgba(252,211,77,0.55)]";
  if (event.type?.includes("tool")) return "bg-emerald-300";
  if (event.type?.includes("model")) return "bg-cyan-300";
  if (event.type?.includes("memory")) return "bg-violet-300";
  return "bg-stone-400";
}

createRoot(document.getElementById("root")).render(
  <React.StrictMode>
    <QueryClientProvider client={queryClient}>
      <App />
    </QueryClientProvider>
  </React.StrictMode>,
);
