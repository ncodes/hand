import { cn } from "../../lib/utils";
import type { ComponentProps } from "react";

export function Badge({ className, ...props }: ComponentProps<"span">) {
  return <span className={cn("rounded-full border px-2.5 py-1 text-xs font-semibold", className)} {...props} />;
}
