-- 0002_work_dir.sql: add the user-set, inheritable working directory to nodes.
--
-- Distinct from workspace_dir (the machine-managed worktree workspace): work_dir
-- is set by the user and inherited by descendants (nearest non-empty ancestor
-- wins), resolved on demand in the tree actor. Empty means "inherit".

ALTER TABLE nodes ADD COLUMN work_dir TEXT NOT NULL DEFAULT '';
