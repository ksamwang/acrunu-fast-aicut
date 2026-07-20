import { requestJSON } from "./http";

export const LOCAL_AGENT_BASE_URL = "http://127.0.0.1:58721";
export const LOCAL_AGENT_APP_ID = "acrunu-fastcut-local-agent";
export const LOCAL_AGENT_PROTOCOL_VERSION = 1;
export const LOCAL_AGENT_LAUNCH_URL = "acrunu-fastcut://launch";

export type LocalAgentHealth = {
  status: "ok";
  app: string;
  version: string;
  protocol_version: number;
  platform: string;
  ffmpeg_ready: boolean;
  ffprobe_ready: boolean;
};

export type LocalAgentProbeResult =
  | { state: "ready"; health: LocalAgentHealth }
  | { state: "unavailable" }
  | { state: "incompatible"; health?: Partial<LocalAgentHealth> }
  | { state: "incomplete"; health: LocalAgentHealth };

export async function probeLocalAgent(timeoutMs = 1500): Promise<LocalAgentProbeResult> {
  const controller = new AbortController();
  const timeout = window.setTimeout(() => controller.abort(), timeoutMs);
  try {
    const response = await fetch(`${LOCAL_AGENT_BASE_URL}/healthz`, {
      cache: "no-store",
      signal: controller.signal
    });
    if (!response.ok) {
      return { state: "unavailable" };
    }
    const health = await response.json() as Partial<LocalAgentHealth>;
    if (
      health.status !== "ok" ||
      health.app !== LOCAL_AGENT_APP_ID ||
      typeof health.protocol_version !== "number"
    ) {
      return { state: "incompatible", health };
    }
    if (health.protocol_version !== LOCAL_AGENT_PROTOCOL_VERSION) {
      return { state: "incompatible", health };
    }
    if (!health.ffmpeg_ready || !health.ffprobe_ready) {
      return { state: "incomplete", health: health as LocalAgentHealth };
    }
    return { state: "ready", health: health as LocalAgentHealth };
  } catch {
    return { state: "unavailable" };
  } finally {
    window.clearTimeout(timeout);
  }
}

export function launchLocalAgent() {
  window.location.href = LOCAL_AGENT_LAUNCH_URL;
}

export async function localAgentRequest<T>(path: string, options: RequestInit = {}): Promise<T> {
  const { response, payload } = await requestJSON(`${LOCAL_AGENT_BASE_URL}${path}`, options);
  if (!response.ok) {
    throw new Error(payload?.error ?? "本地 Agent 请求失败");
  }
  return payload as T;
}
