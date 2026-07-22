import { beforeEach, describe, expect, it } from "vitest";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { ProfilesView } from "./ProfilesView";
import { profileWorld } from "../../mock/profileWorld";

// Runs against the mock API (VITE_MOCK=1) over the shared profileWorld
// singleton; reset before each test so mutations don't leak across cases.
// Queries target roles / unique config paths rather than bare profile names,
// since the add form's DirCombobox lists home directories on mount.
describe("ProfilesView (mock mode)", () => {
  beforeEach(() => {
    profileWorld.reset();
  });

  it("lists the seeded profiles with a protected default", async () => {
    render(<ProfilesView />);

    // "work" is removable; the auto-created default is not.
    expect(await screen.findByRole("button", { name: "Remove work" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Remove default" })).not.toBeInTheDocument();
    // The default profile adopts the CLI's own dir.
    expect(screen.getByText("/Users/daniil/.claude")).toBeInTheDocument();
  });

  it("adds a profile through the form and appends it to the list", async () => {
    render(<ProfilesView />);
    await screen.findByRole("button", { name: "Remove work" });

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "personal" } });
    fireEvent.click(screen.getByRole("button", { name: /add profile/i }));

    // The new profile joins the list (its own remove button + default config dir).
    expect(await screen.findByRole("button", { name: "Remove personal" })).toBeInTheDocument();
    expect(screen.getByText("/Users/daniil/.grove/profiles/claude/personal")).toBeInTheDocument();
  });

  it("surfaces a duplicate-name conflict inline", async () => {
    render(<ProfilesView />);
    await screen.findByRole("button", { name: "Remove work" });

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "work" } });
    fireEvent.click(screen.getByRole("button", { name: /add profile/i }));

    expect(await screen.findByText(/already exists/i)).toBeInTheDocument();
  });

  it("removes a profile after confirming the dialog", async () => {
    render(<ProfilesView />);
    fireEvent.click(await screen.findByRole("button", { name: "Remove work" }));

    const dialog = await screen.findByRole("alertdialog");
    fireEvent.click(within(dialog).getByRole("button", { name: "Remove" }));

    // The remove button (and thus the profile) is gone; the default survives.
    expect(await screen.findByText("/Users/daniil/.claude")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Remove work" })).not.toBeInTheDocument();
  });

  it("runs doctor for a profile and shows its checks", async () => {
    render(<ProfilesView />);

    fireEvent.click(await screen.findByRole("button", { name: "Run doctor for work" }));

    expect(await screen.findByText("config dir resolvable")).toBeInTheDocument();
    expect(screen.getByText("no ANTHROPIC_API_KEY in settings")).toBeInTheDocument();
  });
});
