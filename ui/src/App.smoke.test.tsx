import { afterAll, describe, expect, it } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { AppShell } from "./components/shell/AppShell";
import { WelcomeView } from "./components/node/WelcomeView";
import { NodeView } from "./components/node/NodeView";
import { InboxView } from "./components/inbox/InboxView";
import { startStateSocket, stopStateSocket } from "./state/stateSocketBootstrap";
import { startUsagePolling, stopUsagePolling } from "./state/usagePolling";
import { TASK_UI_PALETTE_ID } from "./mock/fixtures";

// End-to-end sanity check in mock mode: boots the same router/store wiring
// main.tsx uses, over jsdom instead of a real browser. Exists specifically
// to catch render-time crashes that unit tests of individual modules can't
// see -- store wiring, route params, and cross-component prop contracts.
describe("App smoke (mock mode)", () => {
  afterAll(() => {
    stopStateSocket();
  });

  it("boots the mock socket and renders the tree rail alongside an idle node", async () => {
    await startStateSocket();

    const router = createMemoryRouter(
      [
        {
          path: "/",
          element: <AppShell />,
          children: [
            { index: true, element: <WelcomeView /> },
            { path: "n/:id", element: <NodeView /> },
            { path: "inbox", element: <InboxView /> },
          ],
        },
      ],
      { initialEntries: [`/n/${TASK_UI_PALETTE_ID}`] },
    );

    render(<RouterProvider router={router} />);

    // Tree rail populated from the mock hello snapshot.
    expect(await screen.findByText("billing-service")).toBeInTheDocument();

    // NodeView header + rail row both show the node's title.
    expect((await screen.findAllByText("Command palette")).length).toBeGreaterThan(0);

    // This node has no session, so the Terminal tab's empty state renders
    // (and XtermHost/xterm never mounts, keeping this test browser-free).
    expect(screen.getByText("No active session")).toBeInTheDocument();

    // Mobile nav chrome (always mounted, CSS-hidden by breakpoint) renders
    // without crashing -- BottomTabs, MobileTopBar's hamburger.
    expect(screen.getByLabelText("Open tree")).toBeInTheDocument();
    expect(screen.getByText("Tree")).toBeInTheDocument();
  });

  it("usage meter loads mock profiles and cycles 5h <-> week on click", async () => {
    startUsagePolling();

    const router = createMemoryRouter(
      [{ path: "/", element: <AppShell />, children: [{ index: true, element: <WelcomeView /> }] }],
      { initialEntries: ["/"] },
    );
    render(<RouterProvider router={router} />);

    const meter = await screen.findByTitle("Click to switch to the week view");
    expect(await screen.findByText("personal")).toBeInTheDocument();
    // "work" is the cooling-down profile in the 5h fixture: it renders its
    // rate-limited state (name + message as sibling text nodes), not an
    // isolated name element + bar, so match the whole message instead.
    expect(screen.getByText(/^work: rate-limited, resets in/)).toBeInTheDocument();

    fireEvent.click(meter);
    expect(await screen.findByTitle("Click to switch to the 5h view")).toBeInTheDocument();

    stopUsagePolling();
  });
});
