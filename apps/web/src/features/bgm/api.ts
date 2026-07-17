import { apiRequest } from "../../shared/api/server-api";
import type { BGMTrack, BGMTrackInput } from "../../shared/types/bgm";

export function listBGMTracks(_path: string, token: string, includeInactive = false) {
  return apiRequest<BGMTrack[]>(`/api/bgm-tracks${includeInactive ? "?include_inactive=true" : ""}`, {}, token);
}

export function createBGMTrack(input: BGMTrackInput, audio: File, token: string) {
  const body = new FormData();
  body.set("name", input.name.trim());
  body.set("bpm", input.bpm > 0 ? String(input.bpm) : "");
  body.set("mood", input.mood.trim());
  body.set("tags_json", JSON.stringify(input.tags));
  body.set("status", input.status);
  body.set("audio", audio, audio.name);
  return apiRequest<BGMTrack>("/api/bgm-tracks", { method: "POST", body }, token);
}

export function updateBGMTrack(trackID: string, input: BGMTrackInput, token: string) {
  return apiRequest<BGMTrack>(`/api/bgm-tracks/${encodeURIComponent(trackID)}`, {
    method: "PUT",
    body: JSON.stringify(input)
  }, token);
}

export function archiveBGMTrack(trackID: string, token: string) {
  return apiRequest<BGMTrack>(`/api/bgm-tracks/${encodeURIComponent(trackID)}`, { method: "DELETE" }, token);
}
