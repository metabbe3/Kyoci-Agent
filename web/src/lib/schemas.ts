/**
 * Runtime zod schemas mirroring `lib/types.ts` 1:1.
 *
 * Every schema uses `.passthrough()` so a working endpoint that ships new
 * fields (a forward-compatible backend rollout) never hard-fails validation.
 * Consumers should call {@link safeParse} and treat the parser result as
 * advisory — a parse miss must NOT break a panel that was rendering fine; it
 * only flags drift.
 *
 * The drift guard `scripts/check-schemas.mjs` greps the Go
 * `internal/dashboard/dashboard.go` struct tags against the field names here.
 */

import { z } from "zod";

/** Provider list row (`/api/dashboard/providers`). Mirrors `ProviderSummary`. */
export const ProviderSummarySchema = z
  .object({
    name: z.string(),
    enabled: z.boolean(),
    available: z.boolean(),
    default_model: z.string(),
    base_url: z.string(),
    model_count: z.number(),
  })
  .passthrough();
export type ProviderSummaryParsed = z.infer<typeof ProviderSummarySchema>;

/** Model catalog row (`/api/dashboard/models`). Mirrors `ModelRow`. */
export const ModelRowSchema = z
  .object({
    provider: z.string(),
    id: z.string(),
    context_length: z.number(),
    supports_tools: z.boolean(),
    supports_streaming: z.boolean(),
    supports_images: z.boolean(),
    max_output_tokens: z.number(),
    description: z.string().optional(),
  })
  .passthrough();
export type ModelRowParsed = z.infer<typeof ModelRowSchema>;

/** Per-provider editable config (`/api/dashboard/config`). Mirrors `ProviderConfigDTO`. */
export const ProviderConfigDTOSchema = z
  .object({
    enabled: z.boolean(),
    base_url: z.string(),
    api_key: z.string(),
    default_model: z.string(),
    timeout: z.number(),
    max_retries: z.number(),
  })
  .passthrough();
export type ProviderConfigDTOParsed = z.infer<typeof ProviderConfigDTOSchema>;

/** Auto-detected host specs (`/api/dashboard/hardware`). Mirrors `HardwareSpecs`. */
export const HardwareSpecsSchema = z
  .object({
    os: z.string(),
    arch: z.string(),
    chip_model: z.string(),
    cpu_count: z.number(),
    ram_gb: z.number(),
    gpu_model: z.string(),
    vram_gb: z.number(),
    hostname: z.string(),
    is_mac: z.boolean(),
    is_apple_silicon: z.boolean(),
    warnings: z.array(z.string()).optional(),
  })
  .passthrough();
export type HardwareSpecsParsed = z.infer<typeof HardwareSpecsSchema>;

/** A single local-model recommendation. Mirrors `RecommendPick`. */
export const RecommendPickSchema = z
  .object({
    model: z.string(),
    provider: z.string(),
    context_len: z.number(),
    reason: z.string(),
    verdict: z.enum(["fits", "tight", "slow", "too_big"]),
    recommended: z.boolean(),
  })
  .passthrough();
export type RecommendPickParsed = z.infer<typeof RecommendPickSchema>;

/** Cloud guidance block. Mirrors `CloudAdvice`. */
export const CloudAdviceSchema = z
  .object({
    needed: z.boolean(),
    summary: z.string(),
    recommended_providers: z.array(z.string()).optional(),
  })
  .passthrough();
export type CloudAdviceParsed = z.infer<typeof CloudAdviceSchema>;

/** Hardware-fit recommendation bundle (`/api/dashboard/recommendations`). Mirrors `RecommendResult`. */
export const RecommendResultSchema = z
  .object({
    summary: z.string(),
    local: z.array(RecommendPickSchema),
    cloud: CloudAdviceSchema,
  })
  .passthrough();
export type RecommendResultParsed = z.infer<typeof RecommendResultSchema>;

/** A chat-role message. Mirrors `ChatMessage`. */
export const ChatMessageSchema = z.object({
  role: z.enum(["system", "user", "assistant", "tool"]),
  content: z.string(),
});
export type ChatMessageParsed = z.infer<typeof ChatMessageSchema>;

/** An uploaded-file reference. Mirrors `UploadedFile`. */
export const UploadedFileSchema = z.object({
  id: z.string(),
  filename: z.string(),
  size: z.number(),
  mime_type: z.string(),
});
export type UploadedFileParsed = z.infer<typeof UploadedFileSchema>;

/** A streamed chat chunk. Mirrors `SSEChunk`. */
export const SSEChunkSchema = z
  .object({
    content: z.string().optional(),
    done: z.boolean().optional(),
    finish_reason: z.string().optional(),
    error: z.string().optional(),
    usage: z
      .object({
        prompt_tokens: z.number(),
        completion_tokens: z.number(),
        total_tokens: z.number(),
      })
      .optional(),
  })
  .passthrough();
export type SSEChunkParsed = z.infer<typeof SSEChunkSchema>;

/** A zero-AI skill descriptor (`/api/dashboard/skills`). Mirrors `SkillInfo`. */
export const SkillInfoSchema = z
  .object({
    name: z.string(),
    description: z.string(),
    keywords: z.array(z.string()).optional(),
    category: z.string().optional(),
  })
  .passthrough();
export type SkillInfoParsed = z.infer<typeof SkillInfoSchema>;

// ── Top-level response wrappers ──────────────────────────────────────────

export const ProvidersResponseSchema = z
  .object({ providers: z.array(ProviderSummarySchema) })
  .passthrough();

export const ModelsResponseSchema = z
  .object({ models: z.array(ModelRowSchema) })
  .passthrough();

export const ConfigResponseSchema = z
  .object({ providers: z.record(z.string(), ProviderConfigDTOSchema) })
  .passthrough();

export const PutConfigResponseSchema = z
  .object({ ok: z.boolean(), message: z.string() })
  .passthrough();

export const TestConnectionResponseSchema = z
  .object({ available: z.boolean(), error: z.string() })
  .passthrough();

export const SkillsResponseSchema = z
  .object({ skills: z.array(SkillInfoSchema) })
  .passthrough();

// ── Helpers ──────────────────────────────────────────────────────────────

/**
 * Validate a value against a schema without ever throwing. Returns the parsed
 * value on success or `null` on failure (the caller falls back to the raw
 * value). Use this on responses so a schema miss can never break a working
 * endpoint — drift is surfaced by the separate guard script, not at runtime.
 */
export function safeParse<T>(schema: z.ZodType<T>, value: unknown): T | null {
  const result = schema.safeParse(value);
  return result.success ? result.data : null;
}
