import { useCallback, useEffect, useState } from "react";
import clsx from "clsx";
import { AlertTriangle, Check, KeyRound, Plus, Star, Stethoscope, X } from "lucide-react";
import { apiClient } from "../../state/api";
import { DirCombobox } from "../common/DirCombobox";
import { ConfirmDialog } from "../common/ConfirmDialog";
import { EmptyState } from "../common/EmptyState";
import { Pill } from "../common/Pill";
import { FOCUS_RING } from "../../lib/constants";
import type { CreateProfileRequest, DoctorCheck, Profile } from "../../gen/types";

const SUPPORTED_DRIVERS = ["claude", "codex", "gemini", "opencode"];

const INPUT_CLASS =
  "w-full rounded-md border border-border bg-canvas px-2 py-1.5 font-mono text-xs text-ink placeholder:text-ink-faint disabled:opacity-50";

/**
 * Profiles manager (GET/POST /profiles, DELETE /profiles/{id},
 * GET /profiles/{id}/doctor). A profile is a provider account — an isolated CLI
 * config dir a node's sessions run under. Lists the registered accounts, adds
 * new ones, runs per-profile health checks, and removes them (the auto-created
 * default is protected). Owns its own fetch + mutation state; there is no
 * cross-view profiles store to share.
 */
export function ProfilesView() {
  const [profiles, setProfiles] = useState<Profile[] | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);

  const [driver, setDriver] = useState("claude");
  const [name, setName] = useState("");
  const [configDir, setConfigDir] = useState("");
  const [addError, setAddError] = useState<string | null>(null);
  const [adding, setAdding] = useState(false);
  // Remounts DirCombobox after a successful add to clear + refetch it.
  const [resetKey, setResetKey] = useState(0);

  const [pendingRemove, setPendingRemove] = useState<Profile | null>(null);
  const [removingId, setRemovingId] = useState<string | null>(null);
  const [removeError, setRemoveError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const res = await apiClient.getProfiles();
      setProfiles(res.profiles);
      setLoadError(null);
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : String(err));
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  async function addProfile() {
    const trimmedName = name.trim();
    if (!trimmedName || adding) return;
    setAdding(true);
    setAddError(null);
    try {
      const body: CreateProfileRequest = { driver, name: trimmedName };
      if (configDir.trim()) body.config_dir = configDir.trim();
      const created = await apiClient.addProfile(body);
      setProfiles((cur) => [...(cur ?? []), created]);
      setName("");
      setConfigDir("");
      setResetKey((k) => k + 1);
    } catch (err) {
      setAddError(err instanceof Error ? err.message : String(err));
    } finally {
      setAdding(false);
    }
  }

  async function confirmRemove() {
    const profile = pendingRemove;
    if (!profile) return;
    setPendingRemove(null);
    setRemovingId(profile.id);
    setRemoveError(null);
    try {
      await apiClient.deleteProfile(profile.id);
      setProfiles((cur) => (cur ?? []).filter((p) => p.id !== profile.id));
    } catch (err) {
      setRemoveError(err instanceof Error ? err.message : String(err));
    } finally {
      setRemovingId(null);
    }
  }

  return (
    <div className="h-full overflow-y-auto px-5 py-4">
      <div className="mx-auto max-w-2xl space-y-4">
        <div>
          <h1 className="font-sans text-sm font-medium text-ink">Profiles</h1>
          <p className="mt-0.5 font-sans text-2xs text-ink-faint">
            Provider accounts, each an isolated CLI config directory. A node's profile selects which
            account its sessions run under — set it per node and children inherit it.
          </p>
        </div>

        {loadError && (
          <div
            role="alert"
            className="flex items-start gap-2 rounded-md border border-status-failed/40 bg-status-failed/10 px-2.5 py-1.5 text-xs text-status-failed"
          >
            <AlertTriangle size={12} className="mt-0.5 shrink-0" />
            <span className="min-w-0 flex-1 break-words">{loadError}</span>
          </div>
        )}

        {profiles === null && !loadError && <p className="text-xs text-ink-faint">Loading profiles…</p>}

        {profiles !== null && profiles.length === 0 && (
          <EmptyState
            icon={<KeyRound size={26} strokeWidth={1.5} />}
            title="No profiles yet"
            description="Add a provider account below so nodes can run sessions under an isolated CLI config."
          />
        )}

        {profiles !== null && profiles.length > 0 && (
          <ul className="space-y-1.5">
            {profiles.map((profile) => (
              <ProfileRow
                key={profile.id}
                profile={profile}
                removing={removingId === profile.id}
                onRemove={() => setPendingRemove(profile)}
              />
            ))}
          </ul>
        )}

        {removeError && <p className="text-2xs break-words text-status-failed">{removeError}</p>}

        <div className="space-y-2 rounded-md border border-border bg-canvas px-3 py-3">
          <h2 className="text-2xs font-medium tracking-wide text-ink-faint uppercase">Add profile</h2>
          <div className="flex flex-wrap gap-2">
            <div className="min-w-[7rem]">
              <label htmlFor="profile-driver" className="mb-1 block text-2xs text-ink-muted">
                Driver
              </label>
              <select
                id="profile-driver"
                value={driver}
                onChange={(e) => {
                  setDriver(e.target.value);
                  setAddError(null);
                }}
                className={clsx(INPUT_CLASS, FOCUS_RING)}
              >
                {SUPPORTED_DRIVERS.map((d) => (
                  <option key={d} value={d}>
                    {d}
                  </option>
                ))}
              </select>
            </div>
            <div className="min-w-[8rem] flex-1">
              <label htmlFor="profile-name" className="mb-1 block text-2xs text-ink-muted">
                Name
              </label>
              <input
                id="profile-name"
                value={name}
                onChange={(e) => {
                  setName(e.target.value);
                  setAddError(null);
                }}
                placeholder="work"
                spellCheck={false}
                autoComplete="off"
                className={clsx(INPUT_CLASS, FOCUS_RING)}
              />
            </div>
          </div>
          <div>
            <label htmlFor="profile-config-input" className="mb-1 block text-2xs text-ink-muted">
              Config directory (optional)
            </label>
            <DirCombobox
              key={resetKey}
              idPrefix="profile-config"
              value={configDir}
              onChange={(v) => {
                setConfigDir(v);
                setAddError(null);
              }}
              onCommit={() => void addProfile()}
              autoFocus={false}
              placeholder="~/.grove/profiles/<driver>/<name>"
            />
          </div>
          {addError && <p className="text-2xs break-words text-status-failed">{addError}</p>}
          <div className="flex justify-end">
            <button
              type="button"
              onClick={() => void addProfile()}
              disabled={adding || !name.trim()}
              className={clsx(
                "flex items-center gap-1 rounded-md bg-accent px-3 py-1.5 text-xs font-medium text-accent-ink hover:bg-accent-strong disabled:opacity-40",
                FOCUS_RING,
              )}
            >
              <Plus size={13} />
              Add profile
            </button>
          </div>
        </div>
      </div>

      <ConfirmDialog
        open={pendingRemove !== null}
        title={pendingRemove ? `Remove "${pendingRemove.name}"?` : ""}
        description="Nodes using this profile fall back to their inherited account. The account's own credentials and config directory are left untouched."
        confirmLabel="Remove"
        danger
        onConfirm={() => void confirmRemove()}
        onCancel={() => setPendingRemove(null)}
      />
    </div>
  );
}

interface ProfileRowProps {
  profile: Profile;
  removing: boolean;
  onRemove: () => void;
}

/** One profile in the list, with an inline doctor expander. The default profile
 *  cannot be removed (it is re-seeded by the daemon regardless). */
function ProfileRow({ profile, removing, onRemove }: ProfileRowProps) {
  const [doctorOpen, setDoctorOpen] = useState(false);
  const [checks, setChecks] = useState<DoctorCheck[] | null>(null);
  const [doctorError, setDoctorError] = useState<string | null>(null);
  const [doctorLoading, setDoctorLoading] = useState(false);

  async function toggleDoctor() {
    if (doctorOpen) {
      setDoctorOpen(false);
      return;
    }
    setDoctorOpen(true);
    if (checks !== null || doctorLoading) return;
    setDoctorLoading(true);
    setDoctorError(null);
    try {
      const res = await apiClient.profileDoctor(profile.id);
      setChecks(res.checks);
    } catch (err) {
      setDoctorError(err instanceof Error ? err.message : String(err));
    } finally {
      setDoctorLoading(false);
    }
  }

  return (
    <li className="rounded-md border border-border bg-surface-2 px-3 py-2">
      <div className="flex items-center gap-3">
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="truncate font-sans text-xs font-medium text-ink">{profile.name}</span>
            <Pill tone="neutral">{profile.driver}</Pill>
            {profile.is_default && (
              <Pill tone="accent" title="Auto-created; adopts the CLI's own config dir">
                <Star size={9} />
                default
              </Pill>
            )}
          </div>
          <p className="truncate font-mono text-2xs text-ink-faint" title={profile.config_dir}>
            {profile.config_dir}
          </p>
        </div>
        <button
          type="button"
          onClick={() => void toggleDoctor()}
          aria-label={`Run doctor for ${profile.name}`}
          aria-expanded={doctorOpen}
          title="Health checks"
          className={clsx(
            "flex h-7 w-7 shrink-0 items-center justify-center rounded text-ink-faint hover:bg-hover hover:text-ink",
            doctorOpen && "bg-hover text-ink",
            FOCUS_RING,
          )}
        >
          <Stethoscope size={13} />
        </button>
        {!profile.is_default && (
          <button
            type="button"
            onClick={onRemove}
            disabled={removing}
            aria-label={`Remove ${profile.name}`}
            title={`Remove ${profile.name}`}
            className={clsx(
              "flex h-7 w-7 shrink-0 items-center justify-center rounded text-ink-faint hover:bg-hover hover:text-status-failed disabled:opacity-40",
              FOCUS_RING,
            )}
          >
            <X size={13} />
          </button>
        )}
      </div>

      {doctorOpen && (
        <div className="mt-2 border-t border-border pt-2">
          {doctorLoading && <p className="text-2xs text-ink-faint">Running checks…</p>}
          {doctorError && <p className="text-2xs break-words text-status-failed">{doctorError}</p>}
          {checks !== null && (
            <ul className="space-y-1">
              {checks.map((check) => (
                <li key={check.name} className="flex items-start gap-1.5 text-2xs">
                  {check.ok ? (
                    <Check size={12} className="mt-0.5 shrink-0 text-status-done" />
                  ) : (
                    <X size={12} className="mt-0.5 shrink-0 text-status-failed" />
                  )}
                  <span className="min-w-0 flex-1">
                    <span className={clsx(check.ok ? "text-ink-muted" : "text-status-failed")}>
                      {check.name}
                    </span>
                    {check.detail && (
                      <span className="ml-1 break-words text-ink-faint">— {check.detail}</span>
                    )}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </li>
  );
}
