import { afterEach, describe, expect, it } from "vitest";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { ReviewWorkspace } from "./ReviewWorkspace";
import { useReviewWorkspaceStore } from "../../state/reviewWorkspace";
import { HERO_PR_DIR, HERO_PR_NUMBER } from "../../mock/prReviewFixtures";

// Relies on the mock API (VITE_MOCK=1, see vitest.config.ts) and the hero
// PRReview fixture in mock/prReviewFixtures.ts -- mirrors ReviewsView.test.tsx
// and App.smoke.test.tsx's reliance on the same mock layer.
function renderWorkspace() {
  const router = createMemoryRouter([{ path: "/review/:dir/:pr", element: <ReviewWorkspace /> }], {
    initialEntries: [`/review/${encodeURIComponent(HERO_PR_DIR)}/${HERO_PR_NUMBER}`],
  });
  render(<RouterProvider router={router} />);
}

describe("ReviewWorkspace (mock mode)", () => {
  afterEach(() => {
    useReviewWorkspaceStore.getState().reset();
  });

  it("parses and renders the PR's files, hunks, diff lines, and existing threads", async () => {
    renderWorkspace();

    expect(await screen.findByText(/Fix nonce ordering check in TxPool\.Insert/)).toBeInTheDocument();
    expect(screen.getByText("src/Nethermind/Nethermind.TxPool/TxPool.cs")).toBeInTheDocument();
    expect(screen.getByText("src/Nethermind/Nethermind.TxPool.Test/TxPoolTests.cs")).toBeInTheDocument();
    expect(screen.getByText("src/Nethermind/Nethermind.TxPool/TxPoolConfig.cs")).toBeInTheDocument();

    // A specific added line's text, from a specific hunk.
    expect(screen.getByText(/if \(_logger\.IsTrace\)/)).toBeInTheDocument();

    // Existing threads render inline: one unresolved (not mine), one resolved.
    expect(screen.getByText(/Should we also check/)).toBeInTheDocument();
    expect(screen.getByText("Resolved")).toBeInTheDocument();
  });

  it("adding a line comment creates a draft (shown inline and in the drafts rail); removing it clears both", async () => {
    renderWorkspace();
    await screen.findByText("src/Nethermind/Nethermind.TxPool/TxPoolConfig.cs");

    expect(screen.getByText("No drafts yet")).toBeInTheDocument();

    const addButtons = await screen.findAllByRole("button", { name: /add a comment on this line/i });
    fireEvent.click(addButtons[0]);

    const textarea = await screen.findByPlaceholderText("Leave a comment…");
    fireEvent.change(textarea, { target: { value: "Please add a bounds check here." } });
    fireEvent.click(screen.getByRole("button", { name: /add draft/i }));

    // Renders both inline (anchored at the line) and in the pending-drafts rail.
    await waitFor(() => {
      expect(screen.getAllByText("Please add a bounds check here.")).toHaveLength(2);
    });
    expect(screen.queryByText("No drafts yet")).not.toBeInTheDocument();

    const removeButtons = screen.getAllByRole("button", { name: /remove draft/i });
    fireEvent.click(removeButtons[0]);

    await waitFor(() => {
      expect(screen.queryByText("Please add a bounds check here.")).not.toBeInTheDocument();
    });
    expect(await screen.findByText("No drafts yet")).toBeInTheDocument();
  });

  it("Draft with AI fills the composer textarea with the mocked suggestion", async () => {
    renderWorkspace();
    await screen.findByText("src/Nethermind/Nethermind.TxPool/TxPoolConfig.cs");

    const addButtons = await screen.findAllByRole("button", { name: /add a comment on this line/i });
    fireEvent.click(addButtons[0]);

    const textarea = (await screen.findByPlaceholderText("Leave a comment…")) as HTMLTextAreaElement;
    expect(textarea.value).toBe("");

    // SubmitBar has its own always-visible "Draft with AI" for the overall
    // summary, so scope the query to this composer specifically.
    const composer = textarea.closest("div")?.parentElement;
    if (!composer) throw new Error("composer container not found");
    fireEvent.click(within(composer).getByRole("button", { name: /draft with ai/i }));

    await waitFor(() => {
      expect(textarea.value).toContain("Consider logging the rejection reason here");
    });
  });
});
