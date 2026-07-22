import { describe, expect, it } from "vitest";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { RepoPanel } from "./RepoPanel";
import { repoWorld } from "../../../mock/repoWorld";
import { PROJECT_GROVE_ID } from "../../../mock/fixtures";

// Runs against the mock API (VITE_MOCK=1, see vitest.config.ts) over the shared
// repoWorld singleton -- the same mock layer WorkDirChip/ReviewsView tests use.
describe("RepoPanel (mock mode)", () => {
  it("lists the repos registered on a project", async () => {
    render(<RepoPanel projectId={PROJECT_GROVE_ID} />);

    // The seeded grove fixtures appear with their source path and base label.
    expect(await screen.findByText("grove")).toBeInTheDocument();
    expect(screen.getByText("grove-docs")).toBeInTheDocument();
    expect(screen.getByText("/Users/daniil/code/grove")).toBeInTheDocument();
    // default_base "" renders as "auto"; a set base renders verbatim.
    expect(screen.getByText("auto")).toBeInTheDocument();
    expect(screen.getByText("main")).toBeInTheDocument();
  });

  it("shows an empty state for a project with no repos", async () => {
    render(<RepoPanel projectId="proj-empty" />);
    expect(await screen.findByText("No repositories yet")).toBeInTheDocument();
  });

  it("adds a repository through the form and appends it to the list", async () => {
    const projectId = "proj-add-flow";
    render(<RepoPanel projectId={projectId} />);

    // Starts empty.
    expect(await screen.findByText("No repositories yet")).toBeInTheDocument();

    // Type an absolute source path; the name auto-fills from the basename.
    fireEvent.change(screen.getByRole("combobox"), { target: { value: "/srv/code/payments" } });
    expect(screen.getByLabelText("Name")).toHaveValue("payments");

    fireEvent.click(screen.getByRole("button", { name: /add repository/i }));

    // The new repo appears; the empty state is gone.
    expect(await screen.findByText("payments")).toBeInTheDocument();
    expect(screen.queryByText("No repositories yet")).not.toBeInTheDocument();
  });

  it("surfaces a duplicate-name conflict inline", async () => {
    const projectId = "proj-dup-flow";
    repoWorld.add(projectId, { source_path: "/srv/code/dup", name: "dup" });
    render(<RepoPanel projectId={projectId} />);

    expect(await screen.findByText("dup")).toBeInTheDocument();

    fireEvent.change(screen.getByRole("combobox"), { target: { value: "/other/path/dup" } });
    fireEvent.click(screen.getByRole("button", { name: /add repository/i }));

    expect(await screen.findByText(/already registered on this project/i)).toBeInTheDocument();
  });

  it("removes a repository after confirming the dialog", async () => {
    const projectId = "proj-remove-flow";
    repoWorld.add(projectId, { source_path: "/srv/code/doomed", name: "doomed" });
    render(<RepoPanel projectId={projectId} />);

    expect(await screen.findByText("doomed")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Remove doomed" }));

    // The confirmation dialog opens; confirming deletes the repo.
    const dialog = await screen.findByRole("alertdialog");
    fireEvent.click(within(dialog).getByRole("button", { name: "Remove" }));

    expect(await screen.findByText("No repositories yet")).toBeInTheDocument();
    expect(screen.queryByText("doomed")).not.toBeInTheDocument();
  });
});
