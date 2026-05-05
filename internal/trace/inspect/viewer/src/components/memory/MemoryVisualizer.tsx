import { GitBranch, Link2, Network, ShieldCheck, Sparkles } from "lucide-react";
import { compactNumber } from "../../lib/format";
import { buildMemoryGraph, memoryKindClass, memoryKindLabel, memoryStatusClass } from "../../lib/memoryGraph";
import { cn } from "../../lib/utils";
import type { MemoryNode, TraceDetail } from "../../types/trace";
import { Button } from "../ui/button";
import { Card, Panel } from "../ui/card";

type MemoryVisualizerProps = {
  detail?: TraceDetail;
  selectedNodeId?: string | null;
  onSelectNode: (node: MemoryNode) => void;
};

const filters = ["Episodic", "Pinned", "Candidates", "Active", "Tool events", "Decisions", "Outcomes", "Blockers"];

export function MemoryVisualizer({ detail, selectedNodeId, onSelectNode }: MemoryVisualizerProps) {
  const graph = buildMemoryGraph(detail);
  const selectedNode = graph.nodes.find((node) => node.id === selectedNodeId) ?? graph.nodes[0];

  return (
    <div className="space-y-5">
      <section className="grid grid-cols-6 gap-3 max-[1280px]:grid-cols-3 max-[880px]:grid-cols-2 max-[520px]:grid-cols-1">
        <MemoryMetric icon={Network} label="Episodic" value={graph.metrics.episodic} note="Session memory signals" />
        <MemoryMetric icon={Sparkles} label="Candidates" value={graph.metrics.candidates} note="Awaiting admission" />
        <MemoryMetric icon={ShieldCheck} label="Active" value={graph.metrics.active} note="High-confidence records" />
        <MemoryMetric icon={GitBranch} label="Blockers" value={graph.metrics.blockers} note="Failed or unresolved" />
        <MemoryMetric icon={Link2} label="Source Links" value={graph.metrics.sourceLinks} note="Saved source refs" />
        <MemoryMetric icon={Sparkles} label="Confidence" value={`${graph.metrics.confidence}%`} note="Average signal score" />
      </section>

      <Panel className="p-3">
        <div className="flex flex-wrap gap-2">
          {filters.map((filter) => (
            <span key={filter} className="rounded-full border border-cyan-300/20 bg-cyan-300/10 px-3 py-1.5 text-xs font-semibold text-cyan-100">
              {filter}
            </span>
          ))}
        </div>
      </Panel>

      <section className="grid gap-3 xl:grid-cols-[minmax(0,1fr)_22rem]">
        <Panel className="min-h-[34rem] overflow-hidden p-4">
          <div className="flex items-start justify-between gap-3">
            <div>
              <h3 className="text-sm font-semibold">Saved episodic memory map</h3>
              <p className="mt-1 text-xs text-stone-500">
                Persisted memory records returned by the Hand state manager.
              </p>
            </div>
            <span className="rounded-full border border-white/10 bg-white/[0.04] px-2.5 py-1 text-xs text-stone-400">
              {graph.nodes.length} nodes
            </span>
          </div>

          {graph.loadError ? (
            <div className="mt-4 rounded-lg border border-rose-300/20 bg-rose-300/10 p-3 text-xs text-rose-100">
              Saved memory could not be loaded: {graph.loadError}
            </div>
          ) : null}

          {graph.nodes.length ? (
            <div className="relative mt-4 h-[29rem] rounded-lg border border-white/10 bg-[radial-gradient(circle_at_center,rgba(34,211,238,0.12),transparent_42%),linear-gradient(135deg,rgba(255,255,255,0.04),transparent)]">
              <svg className="absolute inset-0 h-full w-full" viewBox="0 0 100 100" preserveAspectRatio="none">
                {graph.links.map((link) => {
                  const from = graph.nodes.find((node) => node.id === link.from);
                  const to = graph.nodes.find((node) => node.id === link.to);
                  if (!from || !to) return null;
                  return (
                    <line
                      key={`${link.from}-${link.to}`}
                      x1={from.x}
                      y1={from.y}
                      x2={to.x}
                      y2={to.y}
                      className="stroke-cyan-200/20"
                      strokeWidth="0.35"
                      vectorEffect="non-scaling-stroke"
                    />
                  );
                })}
              </svg>

              {graph.nodes.map((node) => {
                const active = selectedNode?.id === node.id;
                return (
                  <Button
                    key={node.id}
                    onClick={() => onSelectNode(node)}
                    className={cn(
                      "absolute min-w-[10rem] max-w-[12rem] -translate-x-1/2 -translate-y-1/2 rounded-lg border p-3 text-left shadow-xl shadow-black/20 transition hover:-translate-y-[calc(50%+2px)]",
                      active ? "border-cyan-300/60 bg-cyan-300/15" : "border-white/10 bg-zinc-950/90 hover:border-cyan-300/30",
                    )}
                    style={{ left: `${node.x}%`, top: `${node.y}%` }}
                  >
                    <span className={cn("inline-flex rounded-full border px-2 py-0.5 text-[0.65rem] font-semibold", memoryKindClass(node.kind))}>
                      {memoryKindLabel(node.kind)}
                    </span>
                    <span className="mt-2 block truncate text-xs font-semibold text-stone-100">{node.title}</span>
                    <span className="mt-1 block truncate text-[0.68rem] text-stone-500">{node.traceRef} · {node.confidence}%</span>
                  </Button>
                );
              })}
            </div>
          ) : (
            <div className="mt-4 rounded-lg border border-dashed border-white/10 p-10 text-center text-sm text-stone-500">
              No saved episodic memory records are available for this session.
            </div>
          )}
        </Panel>

        <Panel className="p-4">
          <h3 className="text-sm font-semibold">Memory inspector</h3>
          {selectedNode ? (
            <div className="mt-4 space-y-4">
              <div className="rounded-lg border border-white/10 bg-white/[0.035] p-4">
                <div className="flex flex-wrap gap-2">
                  <span className={cn("rounded-full border px-2.5 py-1 text-xs font-semibold", memoryKindClass(selectedNode.kind))}>
                    {memoryKindLabel(selectedNode.kind)}
                  </span>
                  <span className={cn("rounded-full border px-2.5 py-1 text-xs font-semibold", memoryStatusClass(selectedNode.status))}>
                    {selectedNode.status}
                  </span>
                </div>
                <h4 className="mt-3 text-base font-semibold text-white">{selectedNode.title}</h4>
                <p className="mt-2 text-sm text-stone-400">{selectedNode.text}</p>
              </div>

              <div className="grid grid-cols-2 gap-2">
                <Mini label="Confidence" value={`${selectedNode.confidence}%`} />
                <Mini label="Source" value={selectedNode.sourceRange} />
                <Mini label="Trace ref" value={selectedNode.traceRef} />
                <Mini label="Status" value={selectedNode.status} />
              </div>

              <pre className="max-h-52 overflow-auto rounded-lg border border-white/10 bg-black/25 p-3 text-xs leading-relaxed text-cyan-100">
                {JSON.stringify(selectedNode.metadata, null, 2)}
              </pre>
            </div>
          ) : (
            <div className="mt-4 rounded-lg border border-dashed border-white/10 p-8 text-center text-sm text-stone-500">Select a memory node.</div>
          )}
        </Panel>
      </section>

      <Panel className="p-4">
        <div className="flex items-center justify-between gap-3">
          <h3 className="text-sm font-semibold">Source checkpoints</h3>
          <span className="text-xs text-stone-500">{compactNumber(graph.nodes.length)} saved records</span>
        </div>
        <div className="mt-4 grid gap-2 md:grid-cols-4">
          {graph.nodes.slice(0, 8).map((node) => (
            <Button
              key={node.id}
              onClick={() => onSelectNode(node)}
              className={cn(
                "rounded-lg border p-3 text-left transition",
                selectedNode?.id === node.id ? "border-cyan-300/50 bg-cyan-300/10" : "border-white/10 bg-black/10 hover:border-white/20",
              )}
            >
              <span className="block font-mono text-xs text-stone-500">{node.traceRef}</span>
              <span className="mt-1 block truncate text-xs font-semibold text-stone-200">{node.title}</span>
            </Button>
          ))}
        </div>
      </Panel>
    </div>
  );
}

function MemoryMetric({ icon: Icon, label, value, note }: { icon: typeof Network; label: string; value: string | number; note: string }) {
  return (
    <Card className="p-4 shadow-xl shadow-black/10">
      <div className="flex items-center justify-between gap-3">
        <div className="text-xs font-medium uppercase tracking-[0.16em] text-stone-500">{label}</div>
        <Icon size={16} className="text-cyan-200/70" />
      </div>
      <div className="mt-2 text-2xl font-semibold text-white">{value}</div>
      <div className="mt-1 text-xs text-stone-500">{note}</div>
    </Card>
  );
}

function Mini({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border border-white/10 bg-black/20 p-3">
      <div className="text-[0.65rem] uppercase tracking-[0.14em] text-stone-500">{label}</div>
      <div className="mt-1 truncate text-xs font-semibold text-stone-200">{value}</div>
    </div>
  );
}
