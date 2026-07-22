import { afterEach, describe, expect, it } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { NodeView } from "./NodeView";
import { useTreeStore } from "../../state/tree";
import { PROJECT_GROVE_ID } from "../../mock/fixtures";
import type { Node } from "../../gen/types";

function makeNode(id: string, overrides: Partial<Node> = {}): Node {
  return {
    id,
    parent_id: "",
    kind: "task",
    title: id,
    brief: "",
    status: "idle",
    attention: "none",
    attention_reason: "",
    driver: "",
    profile_id: "",
    current_session_id: "",
    workspace_dir: "",
    work_dir: "",
    meta: {},
    position: 0,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

function renderNodeAt(id: string) {
  const router = createMemoryRouter([{ path: "/n/:id", element: <NodeView /> }], {
    initialEntries: [`/n/${id}`],
  });
  return render(<RouterProvider router={router} />);
}

describe("NodeView Repos tab", () => {
  afterEach(() => {
    useTreeStore.getState().reset();
  });

  it("shows a Repos tab for a project node and renders the RepoPanel when selected", async () => {
    const root = makeNode("root", { kind: "workspace" });
    const project = makeNode(PROJECT_GROVE_ID, { kind: "project", parent_id: "root", title: "Grove" });
    useTreeStore.getState().applyHello(2, [root, project], []);

    renderNodeAt(PROJECT_GROVE_ID);

    fireEvent.click(await screen.findByRole("button", { name: "Repos" }));

    // RepoPanel mounted and loaded the seeded fixtures for this project.
    expect(await screen.findByText("Repositories")).toBeInTheDocument();
    expect(await screen.findByText("grove-docs")).toBeInTheDocument();
  });

  it("does not show a Repos tab for a task node", async () => {
    const root = makeNode("root", { kind: "workspace" });
    const task = makeNode("task", { kind: "task", parent_id: "root" });
    useTreeStore.getState().applyHello(2, [root, task], []);

    renderNodeAt("task");

    expect(await screen.findByRole("button", { name: "Terminal" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Repos" })).not.toBeInTheDocument();
  });
});
