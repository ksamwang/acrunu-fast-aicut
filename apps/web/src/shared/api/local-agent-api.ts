import { requestJSON } from "./http";

export const LOCAL_AGENT_BASE_URL = "http://127.0.0.1:58721";

export async function localAgentRequest<T>(path: string, options: RequestInit = {}): Promise<T> {
  const { response, payload } = await requestJSON(`${LOCAL_AGENT_BASE_URL}${path}`, options);
  if (!response.ok) {
    throw new Error(payload?.error ?? "本地 Agent 请求失败");
  }
  return payload as T;
}
