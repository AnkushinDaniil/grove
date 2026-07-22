-- 0005_repos_soft_delete.sql: repos gain a soft-delete flag.
--
-- worktrees.repo_id is NOT NULL REFERENCES repos(id): once a repo has ever
-- provisioned a task worktree, a hard DELETE FROM repos violates that FK
-- permanently, even for worktrees whose task was later archived (archiving
-- marks worktree rows removed, it does not delete them — they stay so their
-- checkouts can still be reviewed). DeleteRepo instead sets deleted_at and
-- ListRepos filters it out, so the repo reads as gone from the API while
-- worktree rows keep a valid parent.

ALTER TABLE repos ADD COLUMN deleted_at INTEGER;
