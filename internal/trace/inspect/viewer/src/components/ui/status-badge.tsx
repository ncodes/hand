import { Badge } from "./badge";

type StatusBadgeProps = {
  status?: string;
};

export function StatusBadge({ status }: StatusBadgeProps) {
  const normalized = status || "incomplete";
  const cls = normalized.includes("fail")
    ? "border-red-300/30 bg-red-300/10 text-red-200"
    : normalized.includes("complete")
      ? "border-emerald-300/30 bg-emerald-300/10 text-emerald-200"
      : "border-amber-300/30 bg-amber-300/10 text-amber-200";

  return <Badge className={`capitalize ${cls}`}>{normalized.replaceAll("_", " ")}</Badge>;
}
