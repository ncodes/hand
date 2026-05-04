import type { ComponentProps } from "react";

export function Checkbox(props: ComponentProps<"input">) {
  return <input type="checkbox" className="accent-cyan-300" {...props} />;
}
