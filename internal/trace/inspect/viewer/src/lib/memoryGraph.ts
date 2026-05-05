import type { MemoryGraph, MemoryNode, MemoryNodeKind, MemoryNodeStatus, MemoryRecord, TraceDetail } from "../types/trace";

const layout = [
  [18, 26],
  [42, 16],
  [67, 25],
  [80, 50],
  [61, 72],
  [33, 73],
  [16, 52],
  [49, 46],
];

export function buildMemoryGraph(detail?: TraceDetail): MemoryGraph {
  const nodes = (detail?.memories?.items ?? []).map(nodeFromMemoryRecord).slice(0, 12);

  nodes.forEach((node, index) => {
    const [x, y] = layout[index % layout.length];
    node.x = x;
    node.y = y + Math.floor(index / layout.length) * 4;
  });

  const links = nodes.slice(1).map((node, index) => ({
    from: nodes[index].id,
    to: node.id,
  }));

  const confidence = nodes.length
    ? Math.round(nodes.reduce((sum, node) => sum + node.confidence, 0) / nodes.length)
    : 0;

  return {
    nodes,
    links,
    loadError: detail?.memories?.load_error,
    metrics: {
      episodic: nodes.length,
      candidates: nodes.filter((node) => node.status === "candidate").length,
      active: nodes.filter((node) => node.status === "active").length,
      blockers: nodes.filter((node) => node.kind === "blocker").length,
      sourceLinks: nodes.filter((node) => node.sourceRange !== "saved memory").length,
      confidence,
    },
  };
}

export function memoryKindLabel(kind: MemoryNodeKind): string {
  switch (kind) {
    case "tool_event":
      return "Tool event";
    case "task_trace":
      return "Task trace";
    default:
      return titleCase(kind);
  }
}

export function memoryKindClass(kind: MemoryNodeKind): string {
  switch (kind) {
    case "outcome":
      return "border-emerald-300/35 bg-emerald-300/12 text-emerald-100";
    case "tool_event":
      return "border-cyan-300/35 bg-cyan-300/12 text-cyan-100";
    case "blocker":
      return "border-rose-300/35 bg-rose-300/12 text-rose-100";
    case "task_trace":
      return "border-blue-300/35 bg-blue-300/12 text-blue-100";
    case "milestone":
      return "border-amber-300/35 bg-amber-300/12 text-amber-100";
    case "decision":
      return "border-violet-300/35 bg-violet-300/12 text-violet-100";
    case "summary":
      return "border-stone-300/25 bg-stone-300/10 text-stone-100";
  }
}

export function memoryStatusClass(status: MemoryNodeStatus): string {
  switch (status) {
    case "active":
      return "border-emerald-300/30 bg-emerald-300/10 text-emerald-100";
    case "blocked":
      return "border-rose-300/30 bg-rose-300/10 text-rose-100";
    case "candidate":
      return "border-cyan-300/30 bg-cyan-300/10 text-cyan-100";
  }
}

function nodeFromMemoryRecord(record: MemoryRecord, index: number): MemoryNode {
  const kind = kindFromMemoryRecord(record);
  const status = statusFromMemoryRecord(record);
  const sourceIndex = sourceIndexFromMemoryRecord(record);
  const title = record.title?.trim() || record.metadata?.candidate_kind || memoryKindLabel(kind);

  return {
    id: record.id,
    kind,
    status,
    title,
    text: record.text?.trim() || "Saved episodic memory.",
    confidence: normalizedConfidence(record.confidence),
    sourceIndex,
    sourceRange: sourceRangeFromMemoryRecord(record),
    traceRef: sourceIndex >= 0 ? `event:${sourceIndex}` : `memory:${index + 1}`,
    x: 0,
    y: 0,
    metadata: {
      id: record.id,
      kind: record.kind || "",
      status: record.status || "",
      ...(record.metadata ?? {}),
    },
  };
}

function kindFromMemoryRecord(record: MemoryRecord): MemoryNodeKind {
  const kind = record.metadata?.candidate_kind || record.tags?.find((tag) => tag !== "episodic" && tag !== "curated") || "";
  switch (kind) {
    case "decision":
      return "decision";
    case "tool_event":
      return "tool_event";
    case "blocker":
      return "blocker";
    case "task_trace":
      return "task_trace";
    case "project_milestone":
    case "milestone":
      return "milestone";
    case "outcome":
      return "outcome";
    default:
      return "summary";
  }
}

function statusFromMemoryRecord(record: MemoryRecord): MemoryNodeStatus {
  if (record.metadata?.outcome_status === "blocked" || record.metadata?.outcome_status === "failed") return "blocked";
  if (record.status === "active") return "active";
  return "candidate";
}

function sourceIndexFromMemoryRecord(record: MemoryRecord): number {
  const traceRef = record.metadata?.available_trace_event_refs?.match(/trace:(\d+)/);
  if (traceRef) return Number(traceRef[1]);

  return -1;
}

function sourceRangeFromMemoryRecord(record: MemoryRecord): string {
  const start = record.metadata?.source_start;
  const end = record.metadata?.source_end;
  if (start && end) return `messages ${start}-${end}`;

  const offsets = record.source_links?.flatMap((link) => link.offsets ?? []) ?? [];
  if (offsets.length) return `offsets ${Math.min(...offsets)}-${Math.max(...offsets)}`;

  return "saved memory";
}

function normalizedConfidence(confidence?: number): number {
  if (!confidence || Number.isNaN(confidence)) return 0;
  if (confidence <= 1) return Math.round(confidence * 100);

  return Math.round(Math.min(confidence, 100));
}

function titleCase(value: string): string {
  return value.replaceAll("_", " ").replace(/\b\w/g, (letter) => letter.toUpperCase());
}
