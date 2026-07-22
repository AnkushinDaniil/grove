import type { CreateProfileRequest, DoctorResponse, Profile } from "../gen/types";
import { ApiError } from "../state/api";
import { ago, HOUR, MIN } from "./fixtures";

const SUPPORTED_DRIVERS = ["claude", "codex", "gemini", "opencode"];

/**
 * Mutable mock analogue of the daemon's `profiles` table. Seeded with the
 * auto-created `default` claude profile (adopting ~/.claude) plus one extra
 * `work` account so the Profiles manager and the node profile picker demo with
 * zero setup. add()/remove() mutate this state, and doctor() returns a couple of
 * representative checks, so the whole flow is exercisable under VITE_MOCK=1.
 * Validation mirrors only what the mock can (supported driver, non-empty name,
 * duplicate (driver, name), absolute config_dir).
 */
function seedProfiles(): Profile[] {
  return [
    {
      id: "profile-default",
      driver: "claude",
      name: "default",
      config_dir: "/Users/daniil/.claude",
      is_default: true,
      created_at: ago(6 * HOUR),
    },
    {
      id: "profile-work",
      driver: "claude",
      name: "work",
      config_dir: "/Users/daniil/.grove/profiles/claude/work",
      is_default: false,
      created_at: ago(90 * MIN),
    },
  ];
}

class MockProfileWorld {
  private profiles: Profile[] = seedProfiles();
  private seq = 0;

  /** Restores the seeded state; used by tests to isolate the shared singleton. */
  reset(): void {
    this.profiles = seedProfiles();
    this.seq = 0;
  }

  list(): Profile[] {
    return [...this.profiles];
  }

  add(body: CreateProfileRequest): Profile {
    const driver = body.driver.trim();
    if (!SUPPORTED_DRIVERS.includes(driver)) {
      throw new ApiError(400, "driver must be one of claude, codex, gemini, opencode");
    }
    const name = body.name.trim();
    if (name === "") {
      throw new ApiError(400, "name must not be empty");
    }
    const configDir = body.config_dir?.trim() ?? "";
    if (configDir !== "" && !configDir.startsWith("/")) {
      throw new ApiError(400, "config_dir must be an absolute path");
    }
    if (this.profiles.some((p) => p.driver === driver && p.name === name)) {
      throw new ApiError(409, `a ${driver} profile named "${name}" already exists`);
    }
    this.seq += 1;
    const profile: Profile = {
      id: `profile-mock-${this.seq}`,
      driver,
      name,
      config_dir: configDir || `/Users/daniil/.grove/profiles/${driver}/${name}`,
      is_default: false,
      created_at: new Date().toISOString(),
    };
    this.profiles = [...this.profiles, profile];
    return profile;
  }

  remove(id: string): void {
    this.profiles = this.profiles.filter((p) => p.id !== id);
  }

  doctor(id: string): DoctorResponse {
    const profile = this.profiles.find((p) => p.id === id);
    if (!profile) throw new ApiError(404, "profile not found");
    return {
      checks: [
        { name: "config dir resolvable", ok: true, detail: profile.config_dir },
        { name: "no ANTHROPIC_API_KEY in settings", ok: true, detail: "" },
        {
          name: `${profile.driver} CLI runs under profile env`,
          ok: true,
          detail: `CLAUDE_CONFIG_DIR=${profile.config_dir}`,
        },
      ],
    };
  }
}

export const profileWorld = new MockProfileWorld();
