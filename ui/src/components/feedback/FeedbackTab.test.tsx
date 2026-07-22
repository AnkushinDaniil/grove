import { afterAll, afterEach, describe, expect, it } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { FeedbackTab } from "./FeedbackTab";
import { NodeView } from "../node/NodeView";
import { apiClient } from "../../state/api";
import { useFeedbackStore } from "../../state/feedback";
import { startStateSocket, stopStateSocket } from "../../state/stateSocketBootstrap";
import { TASK_UI_ID } from "../../mock/fixtures";

// Relies on the mock API (VITE_MOCK=1, see vitest.config.ts) and its fixed
// feedbackWorld.ts seed data, same pattern ReviewsView.test.tsx uses.
describe("Feedback loop (mock mode)", () => {
  afterEach(() => {
    useFeedbackStore.getState().reset();
  });
  afterAll(() => {
    stopStateSocket();
  });

  function renderFeedbackTab() {
    const router = createMemoryRouter(
      [
        { path: "/stats", element: <FeedbackTab /> },
        { path: "/n/:id", element: <div data-testid="node-view" /> },
      ],
      { initialEntries: ["/stats"] },
    );
    render(<RouterProvider router={router} />);
  }

  it("lists the seeded feedback under the default 'open' filter, excluding the already-resolved item", async () => {
    renderFeedbackTab();

    expect(await screen.findByText(/flagged three golden-fixture mismatches/i)).toBeInTheDocument();
    expect(screen.getByText(/missed an obvious off-by-one/i)).toBeInTheDocument();
    expect(screen.queryByText(/rewrote a working component from scratch/i)).not.toBeInTheDocument();
  });

  it("creating feedback from a node's header persists it through the mock API", async () => {
    await startStateSocket();
    const router = createMemoryRouter([{ path: "/n/:id", element: <NodeView /> }], {
      initialEntries: [`/n/${TASK_UI_ID}`],
    });
    render(<RouterProvider router={router} />);

    fireEvent.click(await screen.findByLabelText("Report feedback"));
    fireEvent.change(screen.getByLabelText("Feedback comment"), {
      target: { value: "Left the terminal in a weird state after a resize." },
    });
    fireEvent.click(screen.getByRole("button", { name: /send feedback/i }));

    expect(await screen.findByText(/thanks -- feedback sent/i)).toBeInTheDocument();

    const open = await apiClient.listFeedback("open");
    expect(open.some((f) => f.comment === "Left the terminal in a weird state after a resize." && f.node_id === TASK_UI_ID)).toBe(true);
  });

  it("resolving an item removes it from the open filter", async () => {
    renderFeedbackTab();
    await screen.findByText(/missed an obvious off-by-one/i);

    // Order-independent: capture whichever item's comment is first (rather
    // than assuming a fixed seed count/order, which the earlier "creating
    // feedback" test already perturbs), then confirm that exact comment is
    // gone from the "open" view once resolved.
    const [resolveButton] = await screen.findAllByRole("button", { name: /^resolve$/i });
    const item = resolveButton.closest("li");
    if (!item) throw new Error("Resolve button rendered outside a list item");
    const commentText = item.querySelector("p")?.textContent ?? "";
    expect(commentText.length).toBeGreaterThan(0);

    fireEvent.click(resolveButton);

    await waitFor(() => {
      expect(screen.queryByText(commentText)).not.toBeInTheDocument();
    });
  });

  it("'Create fix task' spawns a task node and navigates to it", async () => {
    renderFeedbackTab();
    await screen.findByText(/flagged three golden-fixture mismatches/i);

    const [createFixTask] = await screen.findAllByRole("button", { name: /create fix task/i });
    fireEvent.click(createFixTask);

    expect(await screen.findByTestId("node-view")).toBeInTheDocument();
  });
});
