import { requestJSON } from "./http";

export async function apiRequest<T>(path: string, options: RequestInit = {}, token?: string): Promise<T> {
  const { response, payload } = await requestJSON(path, {
    ...options,
    headers: {
      ...(options.body ? { "Content-Type": "application/json" } : {}),
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...options.headers
    }
  });

  if (!response.ok) {
    throw new Error(payload?.error?.message ?? "请求失败");
  }
  return payload.data as T;
}

export function authenticatedApiRequest<T>(path: string, token: string, options: RequestInit = {}) {
  return apiRequest<T>(path, options, token);
}
