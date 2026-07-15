import type { VoiceProfile } from "../../shared/types/voice";

const storageKey = "aicut.voice-profiles.prototype.v1";
export const voiceProfilesChangedEvent = "aicut:voice-profiles-changed";

const sampleText = "这是一段用于确认旁白语速、语气和听感的样音。";

const seedProfiles: VoiceProfile[] = [
  {
    id: "voice-prototype-warm-female",
    name: "温和女声",
    language: "中文",
    style_tags: ["自然", "亲和"],
    reference_text: "希望每一次表达都听起来自然、清晰而有温度。",
    preview_text: sampleText,
    preview_kind: "browser",
    status: "enabled",
    is_default: true,
    created_at: "2026-07-15T00:00:00.000Z",
    updated_at: "2026-07-15T00:00:00.000Z"
  },
  {
    id: "voice-prototype-clear-male",
    name: "清晰男声",
    language: "中文",
    style_tags: ["沉稳", "清晰"],
    reference_text: "用清晰、克制的语气讲清楚每一个重点。",
    preview_text: sampleText,
    preview_kind: "browser",
    status: "enabled",
    is_default: false,
    created_at: "2026-07-15T00:00:00.000Z",
    updated_at: "2026-07-15T00:00:00.000Z"
  },
  {
    id: "voice-prototype-bright-female",
    name: "明快女声",
    language: "中文",
    style_tags: ["轻快", "有活力"],
    reference_text: "用轻快的节奏带出产品使用时的积极感受。",
    preview_text: sampleText,
    preview_kind: "browser",
    status: "enabled",
    is_default: false,
    created_at: "2026-07-15T00:00:00.000Z",
    updated_at: "2026-07-15T00:00:00.000Z"
  }
];

function clone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}

function nowISO() {
  return new Date().toISOString();
}

function normalizeProfiles(input: VoiceProfile[]): VoiceProfile[] {
  const profiles = input.map(
    (profile): VoiceProfile => ({
      ...profile,
      language: profile.language || "中文",
      style_tags: Array.isArray(profile.style_tags) ? profile.style_tags.filter(Boolean) : [],
      reference_text: profile.reference_text || "",
      preview_text: profile.preview_text || sampleText,
      preview_kind: profile.preview_audio_url ? "reference_audio" : profile.preview_kind === "reference_audio" ? "reference_audio" : "browser",
      status: profile.status === "disabled" ? "disabled" : "enabled",
      is_default: Boolean(profile.is_default),
      created_at: profile.created_at || nowISO(),
      updated_at: profile.updated_at || nowISO()
    })
  );
  const enabledProfiles = profiles.filter((profile) => profile.status === "enabled");
  const defaultProfile = enabledProfiles.find((profile) => profile.is_default) ?? enabledProfiles[0];

  return profiles.map((profile) => ({ ...profile, is_default: profile.id === defaultProfile?.id }));
}

function notify() {
  window.dispatchEvent(new Event(voiceProfilesChangedEvent));
}

function persist(profiles: VoiceProfile[]) {
  const normalized = normalizeProfiles(profiles);
  window.localStorage.setItem(storageKey, JSON.stringify(normalized));
  notify();
  return normalized;
}

export function listPrototypeVoiceProfiles(): VoiceProfile[] {
  try {
    const rawValue = window.localStorage.getItem(storageKey);
    if (rawValue === null) {
      return persist(clone(seedProfiles));
    }
    const parsed = JSON.parse(rawValue);
    return Array.isArray(parsed) ? normalizeProfiles(parsed as VoiceProfile[]) : persist(clone(seedProfiles));
  } catch {
    return persist(clone(seedProfiles));
  }
}

export function savePrototypeVoiceProfile(profile: VoiceProfile): VoiceProfile[] {
  const profiles = listPrototypeVoiceProfiles();
  const existingIndex = profiles.findIndex((item) => item.id === profile.id);
  const nextProfiles = [...profiles];
  if (existingIndex >= 0) {
    nextProfiles[existingIndex] = profile;
  } else {
    nextProfiles.unshift(profile);
  }
  return persist(
    profile.is_default
      ? nextProfiles.map((item) => ({ ...item, is_default: item.id === profile.id }))
      : nextProfiles
  );
}

export function deletePrototypeVoiceProfile(profileID: string): VoiceProfile[] {
  return persist(listPrototypeVoiceProfiles().filter((profile) => profile.id !== profileID));
}

export function setPrototypeDefaultVoiceProfile(profileID: string): VoiceProfile[] {
  const updatedAt = nowISO();
  return persist(
    listPrototypeVoiceProfiles().map((profile) => ({
      ...profile,
      is_default: profile.id === profileID,
      updated_at: profile.id === profileID ? updatedAt : profile.updated_at
    }))
  );
}

export function createPrototypeVoiceProfileID() {
  return `voice-${crypto.randomUUID()}`;
}
