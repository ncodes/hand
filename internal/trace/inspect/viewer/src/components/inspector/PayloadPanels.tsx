import { highlightJSON, prettyEvent } from "../../lib/jsonHighlight";
import type { TraceEvent, TraceModelRequest, TraceToolInvocation } from "../../types/trace";
import { MiniMetric } from "../ui/mini-metric";

export function PayloadPanel({ event }: { event: TraceEvent }) {
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

function ToolPayload({ tool }: { tool: TraceToolInvocation }) {
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

function ModelRequestPayload({ request }: { request: TraceModelRequest }) {
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

function KeyValuePayload({ payload, title }: { payload?: object; title?: string }) {
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
