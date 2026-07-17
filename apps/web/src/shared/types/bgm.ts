export type BGMTrackStatus = "enabled" | "disabled" | "archived";

export type BGMTrack = {
  id: string;
  name: string;
  file_name: string;
  audio_url: string;
  mime_type: string;
  file_size_bytes: number;
  duration_ms: number;
  sample_rate: number;
  channels: number;
  bpm?: number;
  mood?: string;
  tags: string[];
  status: BGMTrackStatus;
  created_at: string;
  updated_at: string;
};

export type BGMTrackInput = {
  name: string;
  bpm: number;
  mood: string;
  tags: string[];
  status: "enabled" | "disabled";
};

export type BGMSelection = {
  mode: "random" | "track" | "none";
  track_id: string;
  gain_db: number;
};

export type ResolvedBGM = {
  track_id: string;
  name: string;
  gain_db: number;
};
