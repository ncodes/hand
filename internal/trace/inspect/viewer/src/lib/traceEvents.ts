import { EVENT_GROUPS } from "../constants/events";
import type { TraceDetail, TraceEvent, TraceMetrics } from "../types/trace";
import { compactNumber, formatDuration } from "./format";

export type TraceSeverity = "info" | "warning" | "failure";

export function summarize(detail?: TraceDetail): TraceMetrics {
  const events = detail?.timeline ?? [];
  const contextEvents = events.map((event) => event.context_event).filter(Boolean);
  const timestamps = events.map((event) => new Date(event.timestamp ?? "").getTime()).filter((value) => !Number.isNaN(value));
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

export function filterTimeline(events: TraceEvent[], activeGroups: Set<string>, severity: string, query: string): TraceEvent[] {
  const lowered = query.trim().toLowerCase();
  return events.filter((event) => {
    const group = EVENT_GROUPS.find((candidate) => candidate.types.includes(event.type ?? ""));
    if (group && !activeGroups.has(group.id)) return false;
    if (severity === "warning" && eventSeverity(event) === "info") return false;
    if (severity === "failure" && eventSeverity(event) !== "failure") return false;
    if (!lowered) return true;
    return JSON.stringify(event).toLowerCase().includes(lowered);
  });
}

export function eventSeverity(event?: TraceEvent | null): TraceSeverity {
  if (!event) return "info";
  if (event.type?.includes("failed") || event.failure || failureText(event)) return "failure";
  if (event.type?.includes("warning") || event.workspace_rules) return "warning";
  return "info";
}

export function failureText(event: TraceEvent): boolean {
  const content = event.tool_invocation?.content?.toLowerCase() ?? "";
  return content.includes("error") || content.includes("failed");
}

export function eventPreview(event: TraceEvent): string {
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
