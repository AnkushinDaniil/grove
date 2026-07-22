import { useCallback, useEffect, useState } from "react";
import { apiClient } from "../state/api";
import type { Profile } from "../gen/types";

/** Fetches the profile list (GET /profiles) once per mount. The node profile
 *  picker uses it for its pick list, and the profile chip uses it to turn a
 *  node's profile_id into a display name. Reading also seeds the daemon's
 *  default profile on first run. */
export function useProfiles(): {
  profiles: Profile[] | null;
  error: string | null;
  reload: () => Promise<void>;
} {
  const [profiles, setProfiles] = useState<Profile[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  const reload = useCallback(async () => {
    try {
      const res = await apiClient.getProfiles();
      setProfiles(res.profiles);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }, []);

  useEffect(() => {
    void reload();
  }, [reload]);

  return { profiles, error, reload };
}
