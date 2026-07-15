import { useEffect, useRef, useState } from "react";
import type { VoiceProfile } from "../../shared/types/voice";

export function useVoicePreview() {
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const [playingProfileID, setPlayingProfileID] = useState<string | null>(null);

  const stopPreview = () => {
    if (audioRef.current) {
      audioRef.current.pause();
      audioRef.current.currentTime = 0;
      audioRef.current = null;
    }
    setPlayingProfileID(null);
  };

  const togglePreview = async (profile: VoiceProfile) => {
    if (playingProfileID === profile.id) {
      stopPreview();
      return;
    }

    stopPreview();
    setPlayingProfileID(profile.id);
    if (!profile.preview_audio_url || profile.preview_status !== "ready") {
      setPlayingProfileID(null);
      throw new Error("音色样音尚未生成完成");
    }
    const audio = new Audio(profile.preview_audio_url);
    audioRef.current = audio;
    audio.onended = () => setPlayingProfileID((current) => (current === profile.id ? null : current));
    audio.onerror = () => setPlayingProfileID((current) => (current === profile.id ? null : current));
    try {
      await audio.play();
    } catch (error) {
      setPlayingProfileID(null);
      throw error;
    }
  };

  useEffect(() => stopPreview, []);

  return { playingProfileID, stopPreview, togglePreview };
}
