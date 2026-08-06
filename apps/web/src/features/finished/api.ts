import { apiRequest } from "../../shared/api/server-api";
import type { FinishedWork, FinishedWorkClipCandidates, FinishedWorkClipReplacement, VoiceoverReplacement } from "../../shared/types/generation";

export type FinishedWorkDownloadBatch = {
  download_url: string;
  file_name: string;
  file_count: number;
  expires_at: string;
};

export function createFinishedWorkDownload(workIDs: string[], token: string) {
  return apiRequest<FinishedWorkDownloadBatch>("/api/workbench/works/download-batches", {
    method: "POST",
    body: JSON.stringify({ work_ids: workIDs })
  }, token);
}

export function listFinishedWorkClipCandidates(workID: string, clipID: string, query: string, token: string) {
  const params = new URLSearchParams();
  if (query.trim()) {
    params.set("query", query.trim());
  }
  const suffix = params.size > 0 ? `?${params.toString()}` : "";
  return apiRequest<FinishedWorkClipCandidates>(
    `/api/workbench/works/${encodeURIComponent(workID)}/clips/${encodeURIComponent(clipID)}/candidates${suffix}`,
    {},
    token
  );
}

export function replaceFinishedWorkClips(
  workID: string,
  planUpdatedAt: string,
  replacements: FinishedWorkClipReplacement[],
  token: string
) {
  return apiRequest<FinishedWork>(`/api/workbench/works/${encodeURIComponent(workID)}/clip-replacements`, {
    method: "POST",
    body: JSON.stringify({ plan_updated_at: planUpdatedAt, replacements })
  }, token);
}

export function createVoiceoverReplacement(workID: string, token: string) {
  return apiRequest<VoiceoverReplacement>(`/api/workbench/works/${encodeURIComponent(workID)}/voiceover-replacements`, {
    method: "POST"
  }, token);
}

export function getCurrentVoiceoverReplacement(workID: string, token: string) {
  return apiRequest<VoiceoverReplacement | null>(`/api/workbench/works/${encodeURIComponent(workID)}/voiceover-replacements/current`, {}, token);
}

export function applyVoiceoverReplacement(workID: string, replacementID: string, token: string) {
  return apiRequest<VoiceoverReplacement>(
    `/api/workbench/works/${encodeURIComponent(workID)}/voiceover-replacements/${encodeURIComponent(replacementID)}/apply`,
    { method: "POST" },
    token
  );
}

export function cancelVoiceoverReplacement(workID: string, replacementID: string, token: string) {
  return apiRequest<{ cancelled: boolean }>(
    `/api/workbench/works/${encodeURIComponent(workID)}/voiceover-replacements/${encodeURIComponent(replacementID)}`,
    { method: "DELETE" },
    token
  );
}

export function getFinishedWork(workID: string, token: string) {
  return apiRequest<FinishedWork>(`/api/workbench/works/${encodeURIComponent(workID)}`, {}, token);
}
