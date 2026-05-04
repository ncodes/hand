import { compactNumber } from "../../lib/format";

type MiniMetricProps = {
  label: string;
  value: number;
};

export function MiniMetric({ label, value }: MiniMetricProps) {
  return (
    <div className="rounded-lg border border-white/10 bg-black/20 p-3">
      <div className="text-xs text-stone-500">{label}</div>
      <div className="mt-1 text-lg font-semibold">{compactNumber(value)}</div>
    </div>
  );
}
