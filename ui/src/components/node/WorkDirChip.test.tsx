import { beforeEach, describe, expect, it } from "vitest";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { WorkDirChip } from "./WorkDirChip";
import { useTreeStore } from "../../state/tree";
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

describe("WorkDirChip completion", () => {
  beforeEach(() => {
    useTreeStore.getState().reset();
    const root = makeNode("root", { kind: "workspace" });
    const task = makeNode("task", { kind: "task", parent_id: "root" });
    useTreeStore.getState().applyHello(2, [root, task], []);
  });

  it("opens a combobox and lists directory suggestions from suggestDirs", async () => {
    render(<WorkDirChip nodeId="task" />);

    // Opening the chip mounts the popover, which fetches completions for the
    // empty prefix (the mock's home directory listing).
    fireEvent.click(screen.getByRole("button"));

    const listbox = await screen.findByRole("listbox");
    expect(screen.getByRole("combobox")).toBeInTheDocument();

    // The mock fake tree exposes these top-level home directories.
    expect(within(listbox).getByText("code")).toBeInTheDocument();
    expect(within(listbox).getByText("docs")).toBeInTheDocument();
    // Hidden entries (.config) are not offered for a non-dot prefix.
    expect(within(listbox).queryByText(".config")).not.toBeInTheDocument();
  });

  it("descends into a directory when its row is clicked", async () => {
    render(<WorkDirChip nodeId="task" />);
    fireEvent.click(screen.getByRole("button"));

    const listbox = await screen.findByRole("listbox");
    fireEvent.mouseDown(within(listbox).getByText("code"));

    const input = screen.getByRole<HTMLInputElement>("combobox");
    expect(input.value).toBe("/Users/daniil/code/");
    // Descending refetches: the row for a child directory appears.
    expect(await screen.findByText("grove")).toBeInTheDocument();
  });
});
