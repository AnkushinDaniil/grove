import { useCallback, useEffect, useState } from "react";
import clsx from "clsx";
import { AlertTriangle, FolderGit2, GitBranch, Plus, X } from "lucide-react";
import { apiClient } from "../../../state/api";
import { DirCombobox } from "../../common/DirCombobox";
import { ConfirmDialog } from "../../common/ConfirmDialog";
import { EmptyState } from "../../common/EmptyState";
import { FOCUS_RING } from "../../../lib/constants";
import type { NodeID, Repo } from "../../../gen/types";

/** Basename of an absolute path -- the default repo name (mirrors the daemon's
 *  filepath.Base) shown in the name field until the user overrides it. */
function baseName(path: string): string {
  const trimmed = path.replace(/\/+$/, "");
  const i = trimmed.lastIndexOf("/");
  return i >= 0 ? trimmed.slice(i + 1) : trimmed;
}

const INPUT_CLASS =
  "w-full rounded-md border border-border bg-canvas px-2 py-1.5 font-mono text-xs text-ink placeholder:text-ink-faint disabled:opacity-50";

interface RepoPanelProps {
  projectId: NodeID;
}

/**
 * Repos tab for a project node (GET/POST /projects/{id}/repos, DELETE
 * /repos/{id}). Registering a repo here makes every task created under the
 * project afterwards auto-provision a worktree for it -- which is what unlocks
 * worktree review, merge-back and PR-from-task. Owns its own fetch + mutation
 * state; there is no cross-view repos store to share.
 */
export function RepoPanel({ projectId }: RepoPanelProps) {
  const [repos, setRepos] = useState<Repo[] | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);

  const [sourcePath, setSourcePath] = useState("");
  const [name, setName] = useState("");
  const [nameEdited, setNameEdited] = useState(false);
  const [base, setBase] = useState("");
  const [addError, setAddError] = useState<string | null>(null);
  const [adding, setAdding] = useState(false);
  // Remounts DirCombobox after a successful add to clear + refetch it.
  const [resetKey, setResetKey] = useState(0);

  const [pendingRemove, setPendingRemove] = useState<Repo | null>(null);
  const [removingId, setRemovingId] = useState<string | null>(null);
  const [removeError, setRemoveError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const res = await apiClient.getRepos(projectId);
      setRepos(res.repos);
      setLoadError(null);
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : String(err));
    }
  }, [projectId]);

  useEffect(() => {
    void load();
  }, [load]);

  // The name field auto-fills from the path basename until the user edits it.
  const effectiveName = nameEdited ? name : baseName(sourcePath.trim());

  async function addRepo() {
    const path = sourcePath.trim();
    if (!path || adding) return;
    setAdding(true);
    setAddError(null);
    try {
      const created = await apiClient.addRepo(projectId, {
        source_path: path,
        name: effectiveName.trim() || undefined,
        default_base: base.trim() || undefined,
      });
      setRepos((cur) => [...(cur ?? []), created]);
      setSourcePath("");
      setName("");
      setNameEdited(false);
      setBase("");
      setResetKey((k) => k + 1);
    } catch (err) {
      setAddError(err instanceof Error ? err.message : String(err));
    } finally {
      setAdding(false);
    }
  }

  async function confirmRemove() {
    const repo = pendingRemove;
    if (!repo) return;
    setPendingRemove(null);
    setRemovingId(repo.id);
    setRemoveError(null);
    try {
      await apiClient.deleteRepo(repo.id);
      setRepos((cur) => (cur ?? []).filter((r) => r.id !== repo.id));
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
          <h2 className="font-sans text-sm font-medium text-ink">Repositories</h2>
          <p className="mt-0.5 font-sans text-2xs text-ink-faint">
            Tasks created under this project each get a worktree per repo — the basis for worktree review,
            merge-back, and opening PRs.
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

        {repos === null && !loadError && <p className="text-xs text-ink-faint">Loading repositories…</p>}

        {repos !== null && repos.length === 0 && (
          <EmptyState
            icon={<FolderGit2 size={26} strokeWidth={1.5} />}
            title="No repositories yet"
            description="Register a git repository below so new tasks here get their own worktree, enabling worktree review and PRs."
          />
        )}

        {repos !== null && repos.length > 0 && (
          <ul className="space-y-1.5">
            {repos.map((repo) => (
              <li
                key={repo.id}
                className="flex items-center gap-3 rounded-md border border-border bg-surface-2 px-3 py-2"
              >
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="truncate font-sans text-xs font-medium text-ink">{repo.name}</span>
                    <span className="inline-flex shrink-0 items-center gap-1 rounded border border-border px-1.5 py-0.5 text-2xs text-ink-faint">
                      <GitBranch size={9} />
                      {repo.default_base || "auto"}
                    </span>
                  </div>
                  <p className="truncate font-mono text-2xs text-ink-faint" title={repo.source_path}>
                    {repo.source_path}
                  </p>
                </div>
                <button
                  type="button"
                  onClick={() => setPendingRemove(repo)}
                  disabled={removingId === repo.id}
                  aria-label={`Remove ${repo.name}`}
                  title={`Remove ${repo.name}`}
                  className={clsx(
                    "flex h-7 w-7 shrink-0 items-center justify-center rounded text-ink-faint hover:bg-hover hover:text-status-failed disabled:opacity-40",
                    FOCUS_RING,
                  )}
                >
                  <X size={13} />
                </button>
              </li>
            ))}
          </ul>
        )}

        {removeError && <p className="text-2xs break-words text-status-failed">{removeError}</p>}

        <div className="space-y-2 rounded-md border border-border bg-canvas px-3 py-3">
          <h3 className="text-2xs font-medium tracking-wide text-ink-faint uppercase">Add repository</h3>
          <div>
            <label htmlFor="repo-source-input" className="mb-1 block text-2xs text-ink-muted">
              Source path
            </label>
            <DirCombobox
              key={resetKey}
              idPrefix="repo-source"
              value={sourcePath}
              onChange={(v) => {
                setSourcePath(v);
                setAddError(null);
              }}
              onCommit={() => void addRepo()}
              autoFocus={false}
              placeholder="/absolute/path/to/repo"
            />
          </div>
          <div className="flex flex-wrap gap-2">
            <div className="min-w-[8rem] flex-1">
              <label htmlFor="repo-name-input" className="mb-1 block text-2xs text-ink-muted">
                Name
              </label>
              <input
                id="repo-name-input"
                value={effectiveName}
                onChange={(e) => {
                  setName(e.target.value);
                  setNameEdited(true);
                  setAddError(null);
                }}
                placeholder="repo"
                spellCheck={false}
                autoComplete="off"
                className={clsx(INPUT_CLASS, FOCUS_RING)}
              />
            </div>
            <div className="min-w-[8rem] flex-1">
              <label htmlFor="repo-base-input" className="mb-1 block text-2xs text-ink-muted">
                Base branch (optional)
              </label>
              <input
                id="repo-base-input"
                value={base}
                onChange={(e) => setBase(e.target.value)}
                placeholder="auto-detect"
                spellCheck={false}
                autoComplete="off"
                className={clsx(INPUT_CLASS, FOCUS_RING)}
              />
            </div>
          </div>
          {addError && <p className="text-2xs break-words text-status-failed">{addError}</p>}
          <div className="flex justify-end">
            <button
              type="button"
              onClick={() => void addRepo()}
              disabled={adding || !sourcePath.trim()}
              className={clsx(
                "flex items-center gap-1 rounded-md bg-accent px-3 py-1.5 text-xs font-medium text-accent-ink hover:bg-accent-strong disabled:opacity-40",
                FOCUS_RING,
              )}
            >
              <Plus size={13} />
              Add repository
            </button>
          </div>
        </div>
      </div>

      <ConfirmDialog
        open={pendingRemove !== null}
        title={pendingRemove ? `Remove "${pendingRemove.name}"?` : ""}
        description="New tasks here will no longer get a worktree for this repo. Existing task worktrees are left untouched."
        confirmLabel="Remove"
        danger
        onConfirm={() => void confirmRemove()}
        onCancel={() => setPendingRemove(null)}
      />
    </div>
  );
}
