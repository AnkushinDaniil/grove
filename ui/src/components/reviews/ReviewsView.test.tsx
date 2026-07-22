import { afterEach, describe, expect, it } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { ReviewsView } from "./ReviewsView";
import { useReviewsStore } from "../../state/reviews";

// Relies on the mock API (VITE_MOCK=1, see vitest.config.ts) and its fixed
// reviewFixtures.ts data -- the "mocked getReviews fixture" for this suite,
// mirroring how WorkDirChip.test.tsx and App.smoke.test.tsx lean on the same
// mock layer rather than stubbing the api module per test.
describe("ReviewsView (mock mode)", () => {
  afterEach(() => {
    useReviewsStore.getState().reset();
  });

  it("loads the mock reviews fixture and renders every bucket", async () => {
    const router = createMemoryRouter([{ path: "/reviews", element: <ReviewsView /> }], {
      initialEntries: ["/reviews"],
    });
    render(<RouterProvider router={router} />);

    expect(await screen.findByText("NethermindEth/nethermind")).toBeInTheDocument();

    // needs_review
    expect(screen.getByText("Fix null reference in trie pruning during snap sync")).toBeInTheDocument();
    expect(screen.getByTitle("Checks failing")).toBeInTheDocument();
    // re_review
    expect(screen.getByText("Optimize state sync batch size heuristics")).toBeInTheDocument();
    // reviewed
    expect(screen.getByText("Refactor RLP decoder allocations")).toBeInTheDocument();
    // mine (also the draft PR)
    expect(screen.getByText("WIP: Discovery v5 rate limiting")).toBeInTheDocument();
    expect(screen.getByText("Draft")).toBeInTheDocument();

    // Bucket labels with their counts.
    expect(screen.getByText("Needs review")).toBeInTheDocument();
    expect(screen.getByText("Changed since your review")).toBeInTheDocument();
    expect(screen.getByText("Your PRs")).toBeInTheDocument();
  });

  it("starting a review calls the mock API and navigates to the created node", async () => {
    const router = createMemoryRouter(
      [
        { path: "/reviews", element: <ReviewsView /> },
        { path: "/n/:id", element: <div data-testid="node-view" /> },
      ],
      { initialEntries: ["/reviews"] },
    );
    render(<RouterProvider router={router} />);

    const [firstReviewButton] = await screen.findAllByRole("button", { name: /review in grove/i });
    fireEvent.click(firstReviewButton);

    expect(await screen.findByTestId("node-view")).toBeInTheDocument();
  });
});
