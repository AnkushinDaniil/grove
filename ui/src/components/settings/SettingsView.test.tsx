import { afterEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { SettingsView } from "./SettingsView";
import * as push from "../../state/push";

// The browser Push API plumbing itself is covered by state/push.test.ts;
// this file mocks that module wholesale and only asserts on what
// SettingsView does with each status (copy shown, toggle enabled/checked,
// error/test-button behavior).
describe("SettingsView", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("shows the default message with an off, enabled toggle when not yet subscribed", async () => {
    vi.spyOn(push, "getPushStatus").mockResolvedValue("default");
    render(<SettingsView />);

    expect(await screen.findByText(/get notified when a node needs your attention/i)).toBeInTheDocument();
    const toggle = screen.getByRole("switch", { name: /enable push notifications/i });
    expect(toggle).toHaveAttribute("aria-checked", "false");
    expect(toggle).not.toBeDisabled();
  });

  it("shows a subscribed state with the toggle on and a test-notification button", async () => {
    vi.spyOn(push, "getPushStatus").mockResolvedValue("subscribed");
    render(<SettingsView />);

    expect(await screen.findByRole("switch", { name: /enable push notifications/i })).toHaveAttribute(
      "aria-checked",
      "true",
    );
    expect(screen.getByRole("button", { name: /send test notification/i })).toBeInTheDocument();
  });

  it("does not show the test button before subscribing", async () => {
    vi.spyOn(push, "getPushStatus").mockResolvedValue("default");
    render(<SettingsView />);

    await screen.findByRole("switch", { name: /enable push notifications/i });
    expect(screen.queryByRole("button", { name: /send test notification/i })).not.toBeInTheDocument();
  });

  it("disables the toggle and explains why when notifications are blocked", async () => {
    vi.spyOn(push, "getPushStatus").mockResolvedValue("denied");
    render(<SettingsView />);

    expect(await screen.findByText(/blocked/i)).toBeInTheDocument();
    expect(screen.getByRole("switch", { name: /enable push notifications/i })).toBeDisabled();
  });

  it("explains the insecure-origin case", async () => {
    vi.spyOn(push, "getPushStatus").mockResolvedValue("insecure");
    render(<SettingsView />);

    expect(await screen.findByText(/tailscale https/i)).toBeInTheDocument();
    expect(screen.getByRole("switch", { name: /enable push notifications/i })).toBeDisabled();
  });

  it("explains the unsupported-browser case", async () => {
    vi.spyOn(push, "getPushStatus").mockResolvedValue("unsupported");
    render(<SettingsView />);

    expect(await screen.findByText(/aren't supported in this browser/i)).toBeInTheDocument();
    expect(screen.getByRole("switch", { name: /enable push notifications/i })).toBeDisabled();
  });

  it("calls enablePush when toggled on, then refreshes status", async () => {
    const getStatus = vi
      .spyOn(push, "getPushStatus")
      .mockResolvedValueOnce("default")
      .mockResolvedValueOnce("subscribed");
    const enable = vi.spyOn(push, "enablePush").mockResolvedValue(undefined);
    render(<SettingsView />);

    fireEvent.click(await screen.findByRole("switch", { name: /enable push notifications/i }));

    expect(enable).toHaveBeenCalled();
    expect(await screen.findByRole("switch", { name: /enable push notifications/i })).toHaveAttribute(
      "aria-checked",
      "true",
    );
    expect(getStatus).toHaveBeenCalledTimes(2);
  });

  it("calls disablePush when toggled off from a subscribed state", async () => {
    vi.spyOn(push, "getPushStatus").mockResolvedValue("subscribed");
    const disable = vi.spyOn(push, "disablePush").mockResolvedValue(undefined);
    render(<SettingsView />);

    fireEvent.click(await screen.findByRole("switch", { name: /enable push notifications/i }));
    expect(disable).toHaveBeenCalled();
  });

  it("surfaces an inline error when enabling fails, and leaves the toggle off", async () => {
    vi.spyOn(push, "getPushStatus").mockResolvedValue("default");
    vi.spyOn(push, "enablePush").mockRejectedValue(new Error("Notification permission was not granted."));
    render(<SettingsView />);

    fireEvent.click(await screen.findByRole("switch", { name: /enable push notifications/i }));

    expect(await screen.findByText("Notification permission was not granted.")).toBeInTheDocument();
    expect(screen.getByRole("switch", { name: /enable push notifications/i })).toHaveAttribute(
      "aria-checked",
      "false",
    );
  });

  it("runs the local test notification and reports success", async () => {
    vi.spyOn(push, "getPushStatus").mockResolvedValue("subscribed");
    const test = vi.spyOn(push, "showTestNotification").mockResolvedValue(undefined);
    render(<SettingsView />);

    fireEvent.click(await screen.findByRole("button", { name: /send test notification/i }));

    expect(test).toHaveBeenCalled();
    expect(await screen.findByText(/test notification sent/i)).toBeInTheDocument();
  });

  it("surfaces an error inline when the test notification fails", async () => {
    vi.spyOn(push, "getPushStatus").mockResolvedValue("subscribed");
    vi.spyOn(push, "showTestNotification").mockRejectedValue(new Error("No active service worker."));
    render(<SettingsView />);

    fireEvent.click(await screen.findByRole("button", { name: /send test notification/i }));

    expect(await screen.findByText("No active service worker.")).toBeInTheDocument();
  });
});
