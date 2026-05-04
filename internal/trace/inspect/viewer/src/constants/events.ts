export const POLL_INTERVAL_MS = 3000;

export type EventGroup = {
  id: string;
  label: string;
  color: "cyan" | "green" | "blue" | "violet" | "amber" | "slate" | "red";
  types: string[];
};

export const EVENT_GROUPS: EventGroup[] = [
  { id: "model", label: "Model", color: "cyan", types: ["model.request", "model.response"] },
  { id: "tools", label: "Tools", color: "green", types: ["tool.invocation.started", "tool.invocation.completed"] },
  { id: "context", label: "Context", color: "blue", types: ["context.preflight", "context.postflight.usage_recorded", "context.compaction.triggered", "context.compaction.warning", "context.compaction.pending", "context.compaction.running", "context.compaction.succeeded", "context.compaction.failed"] },
  { id: "memory", label: "Memory", color: "violet", types: ["memory.search.started", "memory.retrieved", "memory.search.failed", "memory.extraction.started", "memory.extraction.window_loaded", "memory.extraction.candidates", "memory.extraction.memory_written", "memory.extraction.completed"] },
  { id: "summary", label: "Summary", color: "amber", types: ["summary.fallback.started", "context.summary.requested", "context.summary.saved", "context.summary.failed", "context.summary.parse_failed", "context.summary.applied"] },
  { id: "plan", label: "Plan", color: "slate", types: ["plan.updated", "plan.cleared", "plan.hydrated"] },
  { id: "errors", label: "Errors", color: "red", types: ["session.failed"] },
];
