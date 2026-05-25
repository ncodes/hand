export type TraceSessionSummary = {
  id: string;
  path?: string;
  model?: string;
  api?: string;
  agent_name?: string;
  final_status?: string;
  started_at?: string;
  updated_at?: string;
  event_count?: number;
};

export type TraceSessionsResponse = {
  sessions?: TraceSessionSummary[];
};

export type TraceDetail = {
  summary?: TraceSessionSummary;
  timeline?: TraceEvent[];
  memories?: SessionMemoryView;
};

export type SessionMemoryView = {
  source?: string;
  items?: MemoryRecord[];
  load_error?: string;
};

export type MemoryRecord = {
  id: string;
  kind?: string;
  status?: string;
  title?: string;
  text?: string;
  tags?: string[];
  metadata?: Record<string, string>;
  source_links?: MemorySourceLink[];
  confidence?: number;
  created_at?: string;
  updated_at?: string;
};

export type MemorySourceLink = {
  session_id?: string;
  message_ids?: number[];
  offsets?: number[];
  summary_id?: string;
  created_by?: string;
  created_reason?: string;
};

export type TraceEvent = {
  index: number;
  type?: string;
  timestamp?: string;
  failure?: unknown;
  generic_payload_raw?: string;
  user_message?: { message?: string };
  final_response?: { message?: string };
  tool_invocation?: TraceToolInvocation;
  model_request?: TraceModelRequest;
  model_response?: TraceModelResponse;
  context_event?: TraceContextEvent;
  summary_event?: TraceSummaryEvent;
  plan_event?: TracePlanEvent;
  workspace_rules?: Record<string, unknown> & {
    truncated_length?: number;
  };
};

export type TraceToolInvocation = {
  name?: string;
  phase?: string;
  pair_index?: number | null;
  input?: string;
  content?: string;
};

export type TraceModelRequest = {
  model?: string;
  context?: {
    instruction_chars?: number;
    message_count?: number;
    message_chars?: number;
    tool_count?: number;
  };
};

export type TraceModelResponse = {
  output_text?: string;
  requires_tool_calls?: boolean;
  tool_calls?: unknown[];
};

export type TraceContextEvent = {
  prompt_tokens?: number;
  total_tokens?: number;
  context_limit?: number;
};

export type TraceSummaryEvent = {
  error?: string;
  source_end_offset?: number;
};

export type TracePlanEvent = {
  summary?: Record<string, unknown> & {
    completed?: number;
    total?: number;
  };
};

export type TraceMetrics = {
  events: number;
  toolCalls: number;
  toolFailures: number;
  modelRequests: number;
  modelResponses: number;
  warnings: number;
  currentTokens: number;
  maxTokens: number;
  contextLimit: number;
  duration: string;
};

export type MemoryNodeKind =
  | "outcome"
  | "tool_event"
  | "blocker"
  | "task_trace"
  | "milestone"
  | "decision"
  | "reflection"
  | "summary";

export type MemoryNodeStatus = "active" | "candidate" | "blocked";

export type MemoryNode = {
  id: string;
  kind: MemoryNodeKind;
  status: MemoryNodeStatus;
  title: string;
  text: string;
  confidence: number;
  sourceIndex: number;
  sourceRange: string;
  traceRef: string;
  x: number;
  y: number;
  metadata: Record<string, string>;
};

export type MemoryGraph = {
  nodes: MemoryNode[];
  links: Array<{ from: string; to: string }>;
  loadError?: string;
  metrics: {
    episodic: number;
    candidates: number;
    active: number;
    blockers: number;
    sourceLinks: number;
    confidence: number;
  };
};
