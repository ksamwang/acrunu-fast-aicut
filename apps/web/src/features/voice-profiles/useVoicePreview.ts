import { useEffect, useRef, useState } from "react";
import type { VoiceProfile } from "../../shared/types/voice";

function speechSettings(profile: VoiceProfile) {
  if (profile.id === "voice-prototype-clear-male") {
    return { pitch: 0.78, rate: 0.94 };
  }
  if (profile.id === "voice-prototype-bright-female") {
    return { pitch: 1.16, rate: 1.08 };
  }
  return { pitch: 1.04, rate: 0.92 };
}

export function useVoicePreview() {
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const [playingProfileID, setPlayingProfileID] = useState<string | null>(null);

  const stopPreview = () => {
    if (audioRef.current) {
      audioRef.current.pause();
      audioRef.current.currentTime = 0;
      audioRef.current = null;
    }
    window.speechSynthesis?.cancel();
    setPlayingProfileID(null);
  };

  const togglePreview = async (profile: VoiceProfile) => {
    if (playingProfileID === profile.id) {
      stopPreview();
      return;
    }

    stopPreview();
    setPlayingProfileID(profile.id);
    if (profile.preview_audio_url) {
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
      return;
    }

    if (!window.speechSynthesis) {
      setPlayingProfileID(null);
      throw new Error("当前浏览器不支持样音播放");
    }
    const utterance = new SpeechSynthesisUtterance(profile.preview_text);
    const settings = speechSettings(profile);
    utterance.lang = profile.language === "中文" ? "zh-CN" : profile.language;
    utterance.pitch = settings.pitch;
    utterance.rate = settings.rate;
    utterance.onend = () => setPlayingProfileID((current) => (current === profile.id ? null : current));
    utterance.onerror = () => setPlayingProfileID((current) => (current === profile.id ? null : current));
    window.speechSynthesis.speak(utterance);
  };

  useEffect(() => stopPreview, []);

  return { playingProfileID, stopPreview, togglePreview };
}
