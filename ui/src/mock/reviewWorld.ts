import type { PR, ReviewRepo, ReviewsResponse } from "../gen/types";
import { buildReviewFixtureRepos, NETHERMIND_DIR, REVIEW_LOGIN } from "./reviewFixtures";

/**
 * Mutable mock analogue of the daemon's `review_dirs` setting. Seeded with
 * the one fixture repo's directory so VITE_MOCK=1 demos the whole tab with
 * zero setup. A watched dir with no matching fixture repo (e.g. one added
 * through the "Manage sources" panel) surfaces as an `errors[]` entry
 * instead of silently vanishing, so that UI path is demoable too.
 */
class MockReviewWorld {
  dirs: string[] = [NETHERMIND_DIR];

  private readonly reposByDir = new Map<string, ReviewRepo>(
    buildReviewFixtureRepos().map((r) => [r.dir, r]),
  );

  reviews(): ReviewsResponse {
    const repos: ReviewRepo[] = [];
    const errors: string[] = [];
    for (const dir of this.dirs) {
      const repo = this.reposByDir.get(dir);
      if (repo) repos.push(repo);
      else errors.push(`couldn't reach ${dir}: not a git repository (mock)`);
    }
    return { login: REVIEW_LOGIN, repos, errors };
  }

  setDirs(dirs: string[]): string[] {
    this.dirs = [...dirs];
    return this.dirs;
  }

  getRepoName(dir: string): string | undefined {
    return this.reposByDir.get(dir)?.name_with_owner;
  }

  findPR(dir: string, number: number): PR | undefined {
    const repo = this.reposByDir.get(dir);
    if (!repo) return undefined;
    for (const bucket of Object.values(repo.buckets)) {
      const found = bucket.find((p: PR) => p.number === number);
      if (found) return found;
    }
    return undefined;
  }
}

export const reviewWorld = new MockReviewWorld();
