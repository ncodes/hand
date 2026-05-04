import { cn } from "../../lib/utils";
import type { ComponentProps } from "react";

export function Input({ className, ...props }: ComponentProps<"input">) {
  return <input className={cn(className)} {...props} />;
}
