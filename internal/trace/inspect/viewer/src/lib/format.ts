export function formatTime(value?: string): string {
  if (!value) return "Unknown";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return String(value);
  return date.toLocaleString();
}

export function formatTimeOnly(value?: string): string {
  if (!value) return "--:--:--";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return String(value);
  return date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

export function formatDuration(ms: number): string {
  if (!Number.isFinite(ms) || ms < 0) return "n/a";
  if (ms < 1000) return `${ms}ms`;
  const seconds = Math.round(ms / 1000);
  if (seconds < 60) return `${seconds}s`;
  return `${Math.floor(seconds / 60)}m ${seconds % 60}s`;
}

export function compactNumber(value?: number | string): string {
  return new Intl.NumberFormat("en", { notation: "compact", maximumFractionDigits: 1 }).format(Number(value || 0));
}
