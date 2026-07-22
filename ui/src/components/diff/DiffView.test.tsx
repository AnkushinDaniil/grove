import { describe, expect, it } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { DiffView } from "./DiffView";
import type { DiffViewFile } from "./types";

// One modest file is enough to exercise the render path -- these tests
// verify grove's own integration (file list -> DiffFileCard, comment
// grouping -> lineAnnotations, composer placement), not @pierre/diffs'
// internal rendering fidelity.
const FILES: DiffViewFile[] = [
  {
    path: "src/example.txt",
    status: "modified",
    additions: 1,
    deletions: 1,
    binary: false,
    original_content: "alpha\nbeta\ngamma\n",
    modified_content: "alpha\nBETA\ngamma\n",
    content_omitted: "",
  },
];

describe("DiffView", () => {
  it("renders a file's diff and an anchored comment annotation", async () => {
    render(
      <DiffView
        files={FILES}
        comments={[{ id: "c1", path: "src/example.txt", side: "RIGHT", line: 2, content: <div>a note on line 2</div> }]}
        viewedScopeKey="test-scope-diff"
        activeComposer={null}
        onOpenComposer={() => {}}
        renderComposer={() => null}
      />,
    );

    // Grove's own file header/toolbar render immediately.
    expect(await screen.findByText("src/example.txt")).toBeInTheDocument();
    expect(screen.getByText("0/1 viewed")).toBeInTheDocument();

    // @pierre/diffs mounts a <diffs-container> custom element and renders
    // the actual diff into its shadow root -- not visible to plain
    // getByText (see pierreTheme.ts / DiffFileCard.tsx comments), so this
    // pierces it directly to confirm real diff content landed.
    const diffsContainer = document.querySelector("diffs-container");
    expect(diffsContainer).toBeTruthy();
    await waitFor(() => {
      expect(diffsContainer?.shadowRoot?.textContent).toContain("BETA");
    });

    // The comment annotation is caller-rendered React content -- @pierre/
    // diffs portals it back out into light DOM, so plain queries see it.
    expect(screen.getByText("a note on line 2")).toBeInTheDocument();
  });

  it("shows an empty state when there are no files", () => {
    render(
      <DiffView
        files={[]}
        comments={[]}
        viewedScopeKey="test-scope-empty"
        activeComposer={null}
        onOpenComposer={() => {}}
        renderComposer={() => null}
        emptyTitle="Nothing changed"
        emptyDescription="No diff to show."
      />,
    );
    expect(screen.getByText("Nothing changed")).toBeInTheDocument();
    expect(screen.getByText("No diff to show.")).toBeInTheDocument();
  });

  it("renders the caller's composer at the active target", () => {
    render(
      <DiffView
        files={FILES}
        comments={[]}
        viewedScopeKey="test-scope-composer"
        activeComposer={{ path: "src/example.txt", side: "RIGHT", line: 2 }}
        onOpenComposer={() => {}}
        renderComposer={(target) => (
          <div>
            composing at {target.path}:{target.line}
          </div>
        )}
      />,
    );
    expect(screen.getByText("composing at src/example.txt:2")).toBeInTheDocument();
  });
});
