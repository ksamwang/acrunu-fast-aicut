export type SystemConfig = {
  key: string;
  value: unknown;
  type: string;
  is_secret: boolean;
  description?: string;
};

export type OpenAICompatibleSettings = {
  provider: string;
  base_url: string;
  api_key_configured: boolean;
  llm_model: string;
  vlm_model: string;
  embedding_model: string;
};

export type RuntimeSettings = {
  llm_max_concurrency: number;
  vlm_max_concurrency: number;
  asr_max_concurrency: number;
  tts_max_concurrency: number;
  render_max_concurrency: number;
  task_max_queued_per_user: number;
  task_max_running_per_user: number;
  vlm_timeout_seconds: number;
  vlm_max_retries: number;
};

export type ModelDiscoveryResult = { models: Array<{ id: string }> };
export type ModelSelectOption = { value: string; label: string };

export type ModelProvider = {
  id: string;
  name: string;
  provider_type: string;
  base_url: string;
  api_key_configured: boolean;
  enabled: boolean;
};

export type ModelCapabilitySetting = {
  capability: string;
  provider_id: string;
  model: string;
  dimension?: number;
};

export type ModelCapabilitySettings = {
  llm: ModelCapabilitySetting;
  vlm: ModelCapabilitySetting;
  embedding: ModelCapabilitySetting;
};
