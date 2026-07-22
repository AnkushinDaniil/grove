import { afterEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { DriverProfileChips } from "./DriverProfileChips";
import { useTreeStore } from "../../state/tree";
import { apiClient } from "../../state/api";
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

describe("DriverProfileChips profile picker (mock mode)", () => {
  afterEach(() => {
    useTreeStore.getState().reset();
    vi.restoreAllMocks();
  });

  it("opens the picker and patches the node's profile_id to the chosen profile", async () => {
    const root = makeNode("root", { kind: "workspace" });
    const task = makeNode("task", { kind: "task", parent_id: "root" });
    useTreeStore.getState().applyHello(2, [root, task], []);
    const patchSpy = vi.spyOn(apiClient, "patchNode").mockResolvedValue(makeNode("task"));

    render(<DriverProfileChips nodeId="task" />);

    // The profile chip is a button; opening it reveals the pick list from
    // GET /profiles (the seeded default + work profiles).
    fireEvent.click(await screen.findByRole("button", { name: "Profile" }));
    fireEvent.click(await screen.findByText("work"));

    expect(patchSpy).toHaveBeenCalledWith("task", { profile_id: "profile-work" });
  });

  it("clears the node's profile to inherit from an ancestor", async () => {
    const root = makeNode("root", { kind: "workspace" });
    const task = makeNode("task", { kind: "task", parent_id: "root", profile_id: "profile-work" });
    useTreeStore.getState().applyHello(2, [root, task], []);
    const patchSpy = vi.spyOn(apiClient, "patchNode").mockResolvedValue(makeNode("task"));

    render(<DriverProfileChips nodeId="task" />);

    fireEvent.click(await screen.findByRole("button", { name: "Profile" }));
    fireEvent.click(await screen.findByText("Inherit from parent"));

    expect(patchSpy).toHaveBeenCalledWith("task", { profile_id: "" });
  });
});
