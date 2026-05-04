import { EVENT_GROUPS } from "../../constants/events";
import { groupClass } from "../../lib/styleMaps";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { Panel } from "../ui/card";
import { Select } from "../ui/select";
import type { Dispatch, SetStateAction } from "react";

type FilterBarProps = {
  query: string;
  onQuery: (query: string) => void;
  activeGroups: Set<string>;
  setActiveGroups: Dispatch<SetStateAction<Set<string>>>;
  severity: string;
  setSeverity: (severity: string) => void;
};

export function FilterBar({ query, onQuery, activeGroups, setActiveGroups, severity, setSeverity }: FilterBarProps) {
  return (
    <Panel className="p-3">
      <div className="flex flex-wrap items-center gap-2 max-[520px]:block max-[520px]:space-y-2">
        <Input
          value={query}
          onChange={(event) => onQuery(event.target.value)}
          placeholder="Search type, payload, tool name, model output..."
          className="min-w-0 flex-1 rounded-lg border border-white/10 bg-black/20 px-3 py-2 text-sm text-stone-100 outline-none placeholder:text-stone-600 focus:border-cyan-300/50 max-[520px]:w-full"
        />
        <Select value={severity} onChange={(event) => setSeverity(event.target.value)} className="rounded-lg border border-white/10 bg-black/20 px-3 py-2 text-sm text-stone-200 outline-none max-[520px]:w-full">
          <option value="all">All severity</option>
          <option value="warning">Warnings</option>
          <option value="failure">Failures</option>
        </Select>
      </div>
      <div className="mt-3 flex flex-wrap gap-2 max-[520px]:gap-1.5">
        {EVENT_GROUPS.map((group) => {
          const active = activeGroups.has(group.id);
          return (
            <Button
              key={group.id}
              onClick={() => {
                const next = new Set(activeGroups);
                if (active) next.delete(group.id);
                else next.add(group.id);
                setActiveGroups(next);
              }}
              className={`rounded-full border px-3 py-1.5 text-xs font-semibold transition max-[520px]:px-2.5 ${active ? groupClass(group.color) : "border-white/10 bg-white/[0.03] text-stone-500"}`}
            >
              {group.label}
            </Button>
          );
        })}
      </div>
    </Panel>
  );
}
