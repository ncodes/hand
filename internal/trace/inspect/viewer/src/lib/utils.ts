export function cn(...values: Array<string | false | null | undefined | Array<string | false | null | undefined>>): string {
  return values.flat().filter(Boolean).join(" ");
}

export function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value));
}
