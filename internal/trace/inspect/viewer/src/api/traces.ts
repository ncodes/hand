import type { TraceDetail, TraceSessionsResponse } from "../types/trace";

export async function fetchSessions(): Promise<TraceSessionsResponse> {
  const response = await fetch("/api/sessions");
  if (!response.ok) throw new Error("failed to fetch trace sessions");
  return response.json();
}

export async function fetchSession(id: string): Promise<TraceDetail> {
  const response = await fetch(`/api/sessions/${encodeURIComponent(id)}`);
  if (!response.ok) throw new Error("failed to fetch trace session");
  return response.json();
}
