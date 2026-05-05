import { useQuery } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { fetchSession, fetchSessions } from "../api/traces";
import { ChartsPanel } from "../components/dashboard/ChartsPanel";
import { FilterBar } from "../components/dashboard/FilterBar";
import { MetricGrid } from "../components/dashboard/MetricGrid";
import { TimelinePanel } from "../components/dashboard/TimelinePanel";
import { Header } from "../components/layout/Header";
import { Inspector } from "../components/layout/Inspector";
import { MobileTopBar } from "../components/layout/MobileTopBar";
import { SessionSidebar } from "../components/layout/SessionSidebar";
import { MemoryVisualizer } from "../components/memory/MemoryVisualizer";
import { EVENT_GROUPS, POLL_INTERVAL_MS } from "../constants/events";
import { useLocalStorageBool, useLocalStorageNumber } from "../hooks/useLocalStorage";
import { useTransientScrollbars } from "../hooks/useTransientScrollbars";
import { filterTimeline, summarize } from "../lib/traceEvents";
import { clamp } from "../lib/utils";
import type { CSSProperties, PointerEvent as ReactPointerEvent } from "react";

export function App() {
  const [selectedId, setSelectedId] = useState("");
  const [selectedEventIndex, setSelectedEventIndex] = useState<number | null>(null);
  const [autoRefresh, setAutoRefresh] = useLocalStorageBool("trace:auto-refresh", true);
  const [singleEventMode, setSingleEventMode] = useLocalStorageBool("trace:single-event", true);
  const [sidebarCollapsed, setSidebarCollapsed] = useLocalStorageBool("trace:sidebar-collapsed", false);
  const [sidebarWidth, setSidebarWidth] = useLocalStorageNumber("trace:sidebar-width", 304);
  const [inspectorWidth, setInspectorWidth] = useLocalStorageNumber("trace:inspector-width", 384);
  const [sidebarDrawerOpen, setSidebarDrawerOpen] = useState(false);
  const [inspectorSheetOpen, setInspectorSheetOpen] = useState(false);
  const [expandedCharts, setExpandedCharts] = useState<Set<string>>(() => new Set());
  const [activeView, setActiveView] = useState<"events" | "memory">("events");
  const [selectedMemoryNodeId, setSelectedMemoryNodeId] = useState<string | null>(null);
  const [query, setQuery] = useState("");
  const [activeGroups, setActiveGroups] = useState(() => new Set(EVENT_GROUPS.map((group) => group.id)));
  const [severity, setSeverity] = useState("all");
  const effectiveSidebarWidth = clamp(sidebarWidth, 240, 520);
  const effectiveInspectorWidth = clamp(inspectorWidth, 320, 640);

  useTransientScrollbars();

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

  useEffect(() => {
    function handleKeyDown(event: KeyboardEvent) {
      if (event.defaultPrevented || !["ArrowDown", "ArrowUp"].includes(event.key)) return;
      if (isTypingTarget(event.target) || !filteredTimeline.length) return;

      event.preventDefault();
      const currentIndex = filteredTimeline.findIndex((item) => item.index === selectedEvent?.index);
      const fallbackIndex = event.key === "ArrowDown" ? -1 : filteredTimeline.length;
      const baseIndex = currentIndex >= 0 ? currentIndex : fallbackIndex;
      const nextIndex = clamp(baseIndex + (event.key === "ArrowDown" ? 1 : -1), 0, filteredTimeline.length - 1);
      setSelectedEventIndex(filteredTimeline[nextIndex].index);
    }

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [filteredTimeline, selectedEvent]);

  function startSidebarResize(event: ReactPointerEvent<HTMLButtonElement>) {
    if (sidebarCollapsed) return;
    event.preventDefault();
    const startX = event.clientX;
    const startWidth = effectiveSidebarWidth;

    function handleMove(moveEvent: PointerEvent) {
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

  function startInspectorResize(event: ReactPointerEvent<HTMLButtonElement>) {
    event.preventDefault();
    const startX = event.clientX;
    const startWidth = effectiveInspectorWidth;

    function handleMove(moveEvent: PointerEvent) {
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
  } as CSSProperties & Record<string, string>;

  return (
    <main className="h-screen min-h-0 overflow-hidden bg-stone-950 text-stone-100 max-[880px]:h-auto max-[880px]:min-h-screen max-[880px]:overflow-x-hidden max-[880px]:overflow-y-auto">
      <div
        className="trace-shell-grid grid h-screen min-h-0 [grid-template-columns:var(--trace-sidebar-width)_minmax(0,1fr)_var(--trace-inspector-width)] max-[1180px]:[grid-template-columns:var(--trace-sidebar-width-narrow)_minmax(0,1fr)] max-[880px]:block max-[880px]:h-auto max-[880px]:min-h-screen"
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
            setSelectedMemoryNodeId(null);
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
            selectedId={selectedId}
            activeView={activeView}
            autoRefresh={autoRefresh}
            singleEventMode={singleEventMode}
            onActiveView={setActiveView}
            onAutoRefresh={setAutoRefresh}
            onSingleEventMode={setSingleEventMode}
          />
          <div className="space-y-5 p-5 max-[520px]:p-3">
            {activeView === "events" ? (
              <>
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
              </>
            ) : (
              <MemoryVisualizer
                detail={detail}
                selectedNodeId={selectedMemoryNodeId}
                onSelectNode={(node) => {
                  setSelectedMemoryNodeId(node.id);
                  if (node.sourceIndex >= 0) {
                    setSelectedEventIndex(node.sourceIndex);
                  }
                  if (window.innerWidth <= 880) {
                    setInspectorSheetOpen(true);
                  }
                }}
              />
            )}
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

function isTypingTarget(target: EventTarget | null): boolean {
  if (!(target instanceof Element)) return false;
  const tagName = target.tagName.toLowerCase();
  return tagName === "input" || tagName === "textarea" || tagName === "select" || target.isContentEditable;
}
