import { cn } from "../../lib/utils";
import type { ComponentProps } from "react";

export function Select({ className, ...props }: ComponentProps<"select">) {
  return <select className={cn(className)} {...props} />;
}
