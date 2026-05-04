export type TraceSessionSummary = {
  id: string;
  path?: string;
  model?: string;
  api_mode?: string;
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
  maxTokens: number;
  contextLimit: number;
  duration: string;
};
