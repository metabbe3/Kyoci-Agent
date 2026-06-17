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
};

export type SkillInfo = {
  name: string;
  description: string;
  keywords?: string[];
  category?: string;
};
