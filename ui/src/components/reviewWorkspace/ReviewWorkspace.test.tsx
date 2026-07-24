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

/** Clicks the first clickable line-number cell @pierre/diffs has rendered so
 *  far (in whichever file's diff appears first in DOM order), opening
 *  DiffView's comment composer there -- the rich-diff equivalent of the old
 *  hand-rolled hunk table's hover "+" button. @pierre/diffs renders into a
 *  <diffs-container> custom element's shadow root (confirmed via a live
 *  render dump), which plain screen.getByText/getByRole can't see into, so
 *  this reaches in directly. Retries via waitFor since the diff mounts
 *  asynchronously (React.lazy + @pierre/diffs' own async highlight pass). */
async function clickFirstLineNumber() {
  await waitFor(() => {
    const cell = document.querySelector("diffs-container")?.shadowRoot?.querySelector<HTMLElement>("[data-column-number]");
    if (!cell) throw new Error("no clickable line number rendered yet");
    fireEvent.click(cell);
  });
}

describe("ReviewWorkspace (mock mode)", () => {
  afterEach(() => {
    useReviewWorkspaceStore.getState().reset();
  });

  it("parses and renders the PR's files via @pierre/diffs, with existing threads anchored inline", async () => {
    renderWorkspace();

    expect(await screen.findByText(/Fix nonce ordering check in TxPool\.Insert/)).toBeInTheDocument();
    // DiffView is React.lazy-loaded -- await the first file so the dynamic
    // import has settled before asserting on the rest of the tree.
    expect(await screen.findByText("src/Nethermind/Nethermind.TxPool/TxPool.cs")).toBeInTheDocument();
    expect(screen.getByText("src/Nethermind/Nethermind.TxPool.Test/TxPoolTests.cs")).toBeInTheDocument();
    expect(screen.getByText("src/Nethermind/Nethermind.TxPool/TxPoolConfig.cs")).toBeInTheDocument();

    // The modified file's added line actually rendered inside @pierre/diffs'
    // shadow root (see clickFirstLineNumber's doc comment on why this reaches
    // in directly rather than using getByText).
    const containers = document.querySelectorAll("diffs-container");
    expect(containers).toHaveLength(3);
    await waitFor(() => {
      expect(containers[0].shadowRoot?.textContent).toContain("IsTrace");
    });

    // Existing threads render inline: one unresolved (not mine), one resolved.
    expect(screen.getByText(/Should we also check/)).toBeInTheDocument();
    expect(screen.getByText("Resolved")).toBeInTheDocument();
  });

  it("adding a line comment creates a draft (shown inline and in the drafts rail); removing it clears both", async () => {
    renderWorkspace();
    await screen.findByText("src/Nethermind/Nethermind.TxPool/TxPool.cs");

    expect(screen.getByText("No drafts yet")).toBeInTheDocument();

    await clickFirstLineNumber();

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

  it("Review with AI lists findings in the panel; dismiss drops one", async () => {
    renderWorkspace();
    await screen.findByText("src/Nethermind/Nethermind.TxPool/TxPool.cs");

    // Run the pass from the AI findings panel.
    fireEvent.click(screen.getByRole("button", { name: /review with ai/i }));

    // The mock returns three findings; each is a navigator row in the panel
    // AND a full card inline in the diff at its line (hence the body appears in
    // more than one place -- the point of line-anchoring).
    await screen.findAllByText(/the pool leaks the entry/i);
    const dismissButtons = () => screen.queryAllByRole("button", { name: /dismiss finding/i });
    expect(dismissButtons()).toHaveLength(3);

    // Dismiss one finding -> its row leaves the panel.
    fireEvent.click(dismissButtons()[0]);
    await waitFor(() => expect(dismissButtons()).toHaveLength(2));
  });

  it("after a review, you can chat with the review session and get a reply", async () => {
    renderWorkspace();
    await screen.findByText("src/Nethermind/Nethermind.TxPool/TxPool.cs");

    // Chat is gated on a review having run (there is a session to resume).
    expect(screen.queryByLabelText("Ask the reviewer")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /review with ai/i }));
    const input = await screen.findByLabelText("Ask the reviewer");

    fireEvent.change(input, { target: { value: "why is the first finding a bug?" } });
    fireEvent.click(screen.getByRole("button", { name: /^send$/i }));

    // The question shows immediately, then the resumed session's reply arrives.
    expect(screen.getByText("why is the first finding a bug?")).toBeInTheDocument();
    await screen.findByText(/the pool keeps a stale entry/i);
  });

  it("Draft with AI fills the composer textarea with the mocked suggestion", async () => {
    renderWorkspace();
    await screen.findByText("src/Nethermind/Nethermind.TxPool/TxPool.cs");

    await clickFirstLineNumber();

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
