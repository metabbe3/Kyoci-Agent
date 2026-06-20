export type ProviderSummary = {
  name: string;
  enabled: boolean;
  available: boolean;
  default_model: string;
  base_url: string;
  model_count: number;
};

export type ModelRow = {
  provider: string;
  id: string;
  context_length: number;
  supports_tools: boolean;
  supports_streaming: boolean;
  supports_images: boolean;
  max_output_tokens: number;
  description?: string;
};

export type ProviderConfigDTO = {
  enabled: boolean;
  base_url: string;
  api_key: string;
  default_model: string;
  timeout: number;
  max_retries: number;
};

export type HardwareSpecs = {
  os: string;
  arch: string;
  chip_model: string;
  cpu_count: number;
  ram_gb: number;
  gpu_model: string;
  vram_gb: number;
  hostname: string;
  is_mac: boolean;
  is_apple_silicon: boolean;
  warnings?: string[];
};

export type RecommendPick = {
  model: string;
  provider: string;
  context_len: number;
  reason: string;
  verdict: "fits" | "tight" | "slow" | "too_big";
  recommended: boolean;
};

export type CloudAdvice = {
  needed: boolean;
  summary: string;
  recommended_providers?: string[];
};

export type RecommendResult = {
  summary: string;
  local: RecommendPick[];
  cloud: CloudAdvice;
};

export type ChatMessage = {
  role: "system" | "user" | "assistant" | "tool";
  content: string;
};

export type UploadedFile = {
  id: string;
  filename: string;
  size: number;
  mime_type: string;
};

export type ChatRequest = {
  mode: "chat" | "agent";
  provider?: string;
  model?: string;
  messages: ChatMessage[];
  timeout?: number;
  files?: UploadedFile[];
};

export type SSEChunk = {
  content?: string;
  done?: boolean;
  finish_reason?: string;
  error?: string;
  usage?: {
    prompt_tokens: number;
    completion_tokens: number;
    total_tokens: number;
  };
  /** Structured activity event for the live activity tree UI. */
  activity?: ActivityEvent;
};

/**
 * One row in the live activity tree. Mirrors the Go `kyoci.ActivityEvent`
 * struct 1:1. Grouped by `task_id` — the UI maintains a Map and updates
 * TreeNode state incrementally as events stream in.
 */
export type ActivityEvent = {
  type: "task_start" | "task_progress" | "sub_activity" | "task_complete" | "log";
  task_id: string;
  task_name: string;
  parent_id?: string;
  role?: string;
  provider?: string;
  model?: string;
  tool_name?: string;
  tool_args?: string;
  detail?: string;
  tool_uses?: number;
  tokens_used?: number;
  status?: string;
  timestamp: number;
};

/**
 * Frontend-side tree node — accumulated state for one task row. Built up from
 * a stream of ActivityEvents. The Map<task_id, TreeNode> is the source of
 * truth for the ActivityTree component.
 */
export type TreeNode = {
  taskID: string;
  taskName: string;
  parentID?: string;
  role?: string;
  provider?: string;
  model?: string;
  toolUses: number;
  tokensUsed: number;
  status: "running" | "done" | "error";
  startedAt: number;
  finishedAt?: number;
  subActivities: ActivityEvent[];
  children: TreeNode[];
};

export type SkillInfo = {
  name: string;
  description: string;
  keywords?: string[];
  category?: string;
};
