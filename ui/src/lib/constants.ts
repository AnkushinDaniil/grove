// Shared class fragments and label maps used across components. Centralized
// so the focus ring, status vocabulary, and kind vocabulary render
// identically everywhere instead of drifting per-component.
import type { Attention, FeedbackKind, NodeKind, NodeStatus, SessionStatus } from "../gen/types";

export const FOCUS_RING =
  "outline-none focus-visible:ring-2 focus-visible:ring-accent focus-visible:ring-offset-2 focus-visible:ring-offset-canvas";

export const STATUS_LABEL: Record<NodeStatus, string> = {
  idle: "Idle",
  starting: "Starting",
  running: "Running",
  awaiting_input: "Awaiting input",
  done: "Done",
  failed: "Failed",
  interrupted: "Interrupted",
};

// Statuses whose dot pulses (has an active process doing -- or waiting to
// resume -- work).
export const PULSING_STATUSES: ReadonlySet<NodeStatus> = new Set([
  "starting",
  "running",
  "awaiting_input",
]);

export const SESSION_STATUS_LABEL: Record<SessionStatus, string> = {
  starting: "Starting",
  running: "Running",
  awaiting_input: "Awaiting input",
  exited: "Exited",
  failed: "Failed",
  interrupted: "Interrupted",
};

export const ATTENTION_LABEL: Record<Attention, string> = {
  none: "",
  permission: "Needs permission",
  question: "Has a question",
  done: "Finished",
  error: "Errored",
  review: "Needs review",
};

export const KIND_LABEL: Record<NodeKind, string> = {
  workspace: "Workspace",
  project: "Project",
  task: "Task",
};

export const CHILD_KIND_FOR: Record<NodeKind, NodeKind | null> = {
  workspace: "project",
  project: "task",
  task: "task",
};

export const FEEDBACK_KIND_LABEL: Record<FeedbackKind, string> = {
  skill: "Skill",
  tool: "Tool",
  model: "Model",
  agent: "Agent",
  other: "Other",
};
