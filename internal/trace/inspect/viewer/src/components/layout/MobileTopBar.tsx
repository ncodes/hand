import { PanelLeftOpen, ScanSearch } from "lucide-react";
import { Button } from "../ui/button";

type MobileTopBarProps = {
  onOpenSidebar: () => void;
  onOpenInspector: () => void;
  hasEvent: boolean;
};

export function MobileTopBar({ onOpenSidebar, onOpenInspector, hasEvent }: MobileTopBarProps) {
  return (
    <div className="hidden grid-cols-[1fr_auto_1fr] items-center gap-2 border-b border-white/10 bg-zinc-950/90 px-3 py-3 backdrop-blur max-[880px]:grid">
      <Button
        onClick={onOpenSidebar}
        className="grid h-10 w-10 place-items-center rounded-lg border border-cyan-300/30 bg-cyan-300/10 text-cyan-100"
        aria-label="Open sessions drawer"
        title="Sessions"
      >
        <PanelLeftOpen size={18} strokeWidth={2.2} />
      </Button>
      <div className="justify-self-center text-center text-[0.68rem] font-semibold uppercase tracking-[0.2em] text-cyan-300">Trace Viewer</div>
      <Button
        onClick={onOpenInspector}
        disabled={!hasEvent}
        className="grid h-10 w-10 place-items-center justify-self-end rounded-lg border border-white/10 bg-white/[0.04] text-stone-200 disabled:opacity-40"
        aria-label="Open inspector"
        title="Inspect"
      >
        <ScanSearch size={18} strokeWidth={2.2} />
      </Button>
    </div>
  );
}
