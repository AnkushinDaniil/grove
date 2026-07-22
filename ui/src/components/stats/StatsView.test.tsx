import { afterEach, describe, expect, it } from "vitest";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { StatsView } from "./StatsView";
import { useStatsStore } from "../../state/stats";
import { useFeedbackStore } from "../../state/feedback";

// Relies on the mock API (VITE_MOCK=1, see vitest.config.ts) and its fixed
// statsFixtures.ts/feedbackWorld.ts seed data -- same pattern
// ReviewsView.test.tsx uses for its own fixture-backed assertions.
describe("StatsView (mock mode)", () => {
  afterEach(() => {
    useStatsStore.getState().reset();
    useFeedbackStore.getState().reset();
  });

  function renderStats() {
    const router = createMemoryRouter([{ path: "/stats", element: <StatsView /> }], { initialEntries: ["/stats"] });
    render(<RouterProvider router={router} />);
  }

  it("loads the mock stats fixture and renders KPI tiles alongside the cost-by-day chart", async () => {
    renderStats();

    // KPI tiles from every section, proving the whole StatsResponse made it
    // through the store into the sections that consume it.
    expect(await screen.findByText("Total cost")).toBeInTheDocument();
    expect(screen.getByText("Input tokens")).toBeInTheDocument();
    expect(screen.getByText("Output tokens")).toBeInTheDocument();
    expect(screen.getByText("Active sessions")).toBeInTheDocument();
    expect(screen.getByText("Tasks created")).toBeInTheDocument();
    expect(screen.getByText(/attention wait/i)).toBeInTheDocument();

    // The hero chart: hand-rolled inline SVG (no chart library), with an
    // accessible group name and a real <svg> inside it.
    const chart = screen.getByRole("group", { name: "Cost by day" });
    expect(chart.querySelector("svg")).toBeInTheDocument();

    // Tools table surfaces WebFetch's deliberately high error rate. Scoped
    // to the Tools section specifically -- "WebFetch" also legitimately
    // appears as a feedback subject in the leaderboard below it.
    const toolsSection = screen.getByRole("heading", { name: "Tools" }).parentElement!;
    expect(within(toolsSection).getByText("WebFetch")).toBeInTheDocument();

    // Feedback leaderboard, derived from feedbackWorld's seed: two open
    // "skill/code-review" items sort to the top as "2 open / 2".
    const feedbackSection = screen.getByRole("region", { name: "Feedback" });
    expect(within(feedbackSection).getByText("2 open / 2")).toBeInTheDocument();
  });

  it('the feedback leaderboard\'s "View all" switches to the Feedback tab', async () => {
    renderStats();
    await screen.findByText("Total cost");

    fireEvent.click(screen.getAllByRole("button", { name: /view all/i })[0]);

    // FeedbackTab's own filter control plus a seeded open item's comment.
    expect(await screen.findByRole("button", { name: "Open" })).toBeInTheDocument();
    expect(screen.getByText(/flagged three golden-fixture mismatches/i)).toBeInTheDocument();
  });
});
