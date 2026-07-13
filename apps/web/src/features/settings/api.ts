import { apiRequest } from "../../shared/api/server-api";
import type { ModelCapabilitySettings, ModelDiscoveryResult, ModelProvider, RuntimeSettings } from "../../shared/types/settings";

export const listModelProviders = (path: string, token: string) => apiRequest<ModelProvider[]>(path, {}, token);
export const getModelSettings = (path: string, token: string) => apiRequest<ModelCapabilitySettings>(path, {}, token);
export const getRuntimeSettings = (path: string, token: string) => apiRequest<RuntimeSettings>(path, {}, token);

export function saveModelProvider(providerID: string | undefined, values: unknown, token: string) {
  return apiRequest<ModelProvider>(
    providerID ? `/api/admin/model-providers/${providerID}` : "/api/admin/model-providers",
    { method: providerID ? "PUT" : "POST", body: JSON.stringify(values) },
    token
  );
}

export const deleteModelProvider = (providerID: string, token: string) =>
  apiRequest<{ deleted: boolean }>(`/api/admin/model-providers/${providerID}`, { method: "DELETE" }, token);

export const discoverModels = (providerID: string, token: string) =>
  apiRequest<ModelDiscoveryResult>(`/api/admin/model-providers/${providerID}/models`, { method: "POST" }, token);

export const testModelProvider = (providerID: string, token: string) =>
  apiRequest<{ reachable: boolean; model_count: number }>(`/api/admin/model-providers/${providerID}/test`, { method: "POST" }, token);

export const saveModelSettings = (values: unknown, token: string) =>
  apiRequest<ModelCapabilitySettings>("/api/admin/model-settings", { method: "PUT", body: JSON.stringify(values) }, token);

export const saveRuntimeSettings = (values: unknown, token: string) =>
  apiRequest<RuntimeSettings>("/api/admin/runtime-settings", { method: "PUT", body: JSON.stringify(values) }, token);
