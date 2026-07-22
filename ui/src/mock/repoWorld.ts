import type { CreateRepoRequest, NodeID, Repo } from "../gen/types";
import { ApiError } from "../state/api";
import { ago, MIN } from "./fixtures";
import { PROJECT_GROVE_ID } from "./fixtures";

/** Basename of an absolute path -- the daemon's `filepath.Base` default for an
 *  omitted repo name, mirrored so the mock derives the same name. */
function baseName(path: string): string {
  const trimmed = path.replace(/\/+$/, "");
  const i = trimmed.lastIndexOf("/");
  return i >= 0 ? trimmed.slice(i + 1) : trimmed;
}

/**
 * Mutable mock analogue of the daemon's `repos` table, keyed by project node.
 * Seeded so the grove project demos the Repos tab (list + remove) with zero
 * setup; add()/remove() mutate this state so the add/remove flows are fully
 * exercisable under VITE_MOCK=1. It intentionally validates only what the mock
 * can (absolute path, duplicate name) -- git-work-tree validation is the real
 * daemon's job.
 */
class MockRepoWorld {
  private readonly byProject = new Map<NodeID, Repo[]>();
  private seq = 0;

  constructor() {
    this.byProject.set(PROJECT_GROVE_ID, [
      this.make(PROJECT_GROVE_ID, "grove", "/Users/daniil/code/grove", ""),
      this.make(PROJECT_GROVE_ID, "grove-docs", "/Users/daniil/code/grove-docs", "main"),
    ]);
  }

  private make(projectId: NodeID, name: string, sourcePath: string, defaultBase: string): Repo {
    this.seq += 1;
    return {
      id: `repo-mock-${this.seq}`,
      project_id: projectId,
      name,
      source_path: sourcePath,
      default_base: defaultBase,
      created_at: ago(3 * MIN),
    };
  }

  list(projectId: NodeID): Repo[] {
    return this.byProject.get(projectId) ?? [];
  }

  hasRepos(projectId: NodeID): boolean {
    return this.list(projectId).length > 0;
  }

  add(projectId: NodeID, body: CreateRepoRequest): Repo {
    const sourcePath = body.source_path.trim();
    if (!sourcePath.startsWith("/")) {
      throw new ApiError(400, "source_path must be an absolute path");
    }
    const name = body.name?.trim() || baseName(sourcePath);
    const existing = this.list(projectId);
    if (existing.some((r) => r.name === name)) {
      throw new ApiError(409, `a repo named "${name}" is already registered on this project`);
    }
    const repo = this.make(projectId, name, sourcePath, body.default_base?.trim() ?? "");
    this.byProject.set(projectId, [...existing, repo]);
    return repo;
  }

  remove(repoId: string): void {
    for (const [projectId, repos] of this.byProject) {
      const next = repos.filter((r) => r.id !== repoId);
      if (next.length !== repos.length) this.byProject.set(projectId, next);
    }
  }
}

export const repoWorld = new MockRepoWorld();
