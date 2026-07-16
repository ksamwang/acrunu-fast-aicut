import { apiRequest } from "../../shared/api/server-api";
import type { SubtitleStylePreset, SubtitleStylePresetInput } from "../../shared/types/subtitle";

export const listSubtitleStylePresets = (path: string, token: string) =>
  apiRequest<SubtitleStylePreset[]>(path, {}, token);

export function saveSubtitleStylePreset(presetID: string | undefined, input: SubtitleStylePresetInput, token: string) {
  return apiRequest<SubtitleStylePreset>(
    presetID ? `/api/admin/subtitle-presets/${presetID}` : "/api/admin/subtitle-presets",
    { method: presetID ? "PUT" : "POST", body: JSON.stringify(input) },
    token
  );
}

export const setDefaultSubtitleStylePreset = (presetID: string, token: string) =>
  apiRequest<SubtitleStylePreset>(`/api/admin/subtitle-presets/${presetID}/default`, { method: "POST" }, token);

export const deleteSubtitleStylePreset = (presetID: string, token: string) =>
  apiRequest<{ deleted: boolean }>(`/api/admin/subtitle-presets/${presetID}`, { method: "DELETE" }, token);
