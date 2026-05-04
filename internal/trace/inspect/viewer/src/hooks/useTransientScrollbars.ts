import { useEffect } from "react";

export function useTransientScrollbars(): void {
  useEffect(() => {
    const timers = new Map();

    function handleScroll(event: Event) {
      const target = event.target === document ? document.documentElement : event.target;
      if (!(target instanceof Element)) return;

      target.classList.add("trace-scroll-active");
      const existing = timers.get(target);
      if (existing) window.clearTimeout(existing);

      timers.set(
        target,
        window.setTimeout(() => {
          target.classList.remove("trace-scroll-active");
          timers.delete(target);
        }, 900),
      );
    }

    document.addEventListener("scroll", handleScroll, true);

    return () => {
      document.removeEventListener("scroll", handleScroll, true);
      for (const [target, timer] of timers) {
        window.clearTimeout(timer);
        target.classList.remove("trace-scroll-active");
      }
    };
  }, []);
}
