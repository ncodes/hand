import { cn } from "../../lib/utils";
import type { ComponentProps } from "react";

export function Card({ className, ...props }: ComponentProps<"article">) {
  return <article className={cn("rounded-lg border border-white/10 bg-zinc-900/70", className)} {...props} />;
}

export function Panel({ className, ...props }: ComponentProps<"section">) {
  return <section className={cn("rounded-lg border border-white/10 bg-zinc-900/70", className)} {...props} />;
}
