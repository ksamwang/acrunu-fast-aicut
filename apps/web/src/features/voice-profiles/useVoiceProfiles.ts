import { useEffect, useState } from "react";
import type { VoiceProfile } from "../../shared/types/voice";
import { listVoiceProfiles } from "./api";

const refreshIntervalMs = 3_000;

export function useVoiceProfiles(token: string) {
  const [profiles, setProfiles] = useState<VoiceProfile[]>([]);
  const [loading, setLoading] = useState(true);

  const reload = async () => {
    const nextProfiles = await listVoiceProfiles("/api/voice-profiles", token);
    setProfiles(nextProfiles);
    return nextProfiles;
  };

  useEffect(() => {
    let disposed = false;
    const load = async () => {
      try {
        const nextProfiles = await listVoiceProfiles("/api/voice-profiles", token);
        if (!disposed) {
          setProfiles(nextProfiles);
        }
      } catch {
        // The caller surfaces save actions; background polling should stay quiet.
      } finally {
        if (!disposed) {
          setLoading(false);
        }
      }
    };
    void load();
    const timer = window.setInterval(() => void load(), refreshIntervalMs);
    return () => {
      disposed = true;
      window.clearInterval(timer);
    };
  }, [token]);

  return { profiles, loading, reload };
}
