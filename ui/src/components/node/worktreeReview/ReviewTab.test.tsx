import { afterEach, describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { ReviewTab } from "./ReviewTab";
import { useWorktreeReviewStore } from "../../../state/worktreeReview";
import { buildFixtureNodes, TASK_PG17_ID, TASK_STRIPE_ID } from "../../../mock/fixtures";

// task-stripe is the hero fixture for worktree review (see
// mock/worktreeReviewFixtures.ts); task-pg17 has a workspace_dir but no
// fixture content, exercising the "clean worktree" empty state.
const stripeNode = buildFixtureNodes().find((n) => n.id === TASK_STRIPE_ID)!;
const cleanNode = buildFixtureNodes().find((n) => n.id === TASK_PG17_ID)!;

describe("ReviewTab (mock mode)", () => {
  afterEach(() => {
    useWorktreeReviewStore.getState().reset();
  });

  it("renders the worktree's diff via DiffView, an existing local comment, and the action bar", async () => {
    render(<ReviewTab node={stripeNode} onAddressed={() => {}} />);

    expect(await screen.findByText("internal/billing/webhook.go")).toBeInTheDocument();
    expect(screen.getByText("internal/billing/webhook_test.go")).toBeInTheDocument();

    // The seeded local comment renders as a DiffView annotation.
    expect(screen.getByText(/Should we log when a duplicate event is dropped/)).toBeInTheDocument();

    // Action bar reflects the one seeded comment and offers both actions.
    expect(screen.getByText("1 local comment")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /merge to parent/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /address with agent/i })).toBeInTheDocument();
  });

  it("shows a friendly empty state for a task with a worktree but no changes yet", async () => {
    render(<ReviewTab node={cleanNode} onAddressed={() => {}} />);

    expect(await screen.findByText("No worktree changes to review")).toBeInTheDocument();
  });
});
