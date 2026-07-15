import { useEffect, useState } from "react";
import type { VoiceProfile } from "../../shared/types/voice";
import { listPrototypeVoiceProfiles, voiceProfilesChangedEvent } from "./prototype-store";

export function useVoiceProfiles() {
  const [profiles, setProfiles] = useState<VoiceProfile[]>(() => listPrototypeVoiceProfiles());

  useEffect(() => {
    const reload = () => setProfiles(listPrototypeVoiceProfiles());
    const onStorage = (event: StorageEvent) => {
      if (event.key === "aicut.voice-profiles.prototype.v1") {
        reload();
      }
    };

    window.addEventListener(voiceProfilesChangedEvent, reload);
    window.addEventListener("storage", onStorage);
    return () => {
      window.removeEventListener(voiceProfilesChangedEvent, reload);
      window.removeEventListener("storage", onStorage);
    };
  }, []);

  return profiles;
}
