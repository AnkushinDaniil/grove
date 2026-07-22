import { describe, expect, it } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryTab } from "./MemoryTab";

// Relies on the mock API (VITE_MOCK=1, see vitest.config.ts) and its
// memoryFixtures.ts seed: self scope shows a decision + gotcha; ancestors scope
// folds in a project convention.
describe("MemoryTab (mock mode)", () => {
  it("lists self-scope memory with kind badges and the backend name", async () => {
    render(<MemoryTab nodeId="task-mem" />);

    expect(await screen.findByText(/Chose Postgres over SQLite/)).toBeInTheDocument();
    expect(screen.getByText(/ACCESS EXCLUSIVE lock/)).toBeInTheDocument();
    // Kind badges render.
    expect(screen.getByText("Decision")).toBeInTheDocument();
    expect(screen.getByText("Gotcha")).toBeInTheDocument();
    // Healthy backend is surfaced.
    expect(screen.getByText("mempalace")).toBeInTheDocument();
  });

  it("switching scope to Ancestors reveals inherited project conventions", async () => {
    render(<MemoryTab nodeId="task-mem" />);

    // The convention lives in an ancestor room, not in self scope.
    await screen.findByText(/Chose Postgres over SQLite/);
    expect(screen.queryByText("Convention")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Ancestors" }));

    expect(await screen.findByText(/never a bare payload/)).toBeInTheDocument();
    expect(screen.getByText("Convention")).toBeInTheDocument();
  });
});
