import "@testing-library/jest-dom/vitest";
import { afterEach } from "vitest";
import { cleanup } from "@testing-library/react";

// @testing-library/react's auto-cleanup only self-registers when it finds
// afterEach on globalThis, which requires vitest's `test.globals: true`.
// This project imports test globals explicitly instead, so cleanup has to
// be wired by hand -- otherwise one test's DOM (and its store-subscribed
// components) leaks into the next, causing bizarre "multiple elements
// found" failures that have nothing to do with the test actually failing.
afterEach(cleanup);

// jsdom doesn't implement layout/scroll APIs at all -- these are used by
// TreeNodeRow (keyboard-nav scroll-into-view) and are correct in real
// browsers; stub them here rather than guarding every call site in app code.
if (!Element.prototype.scrollIntoView) {
  Element.prototype.scrollIntoView = () => {};
}
