import PierreDiffsWorker from "@pierre/diffs/worker/worker.js?worker";
import { registerCustomTheme } from "@pierre/diffs";
import type { ThemeRegistration, ThemesType } from "@pierre/diffs";

/** Worker factory for WorkerPoolContextProvider -- offloads syntax
 *  highlighting off the main thread. Vite handles the `?worker` import
 *  natively in real builds (confirmed: `npm run build` emits a genuine
 *  worker-*.js chunk), bundling worker.js as a separate chunk. */
export function createDiffsWorker(): Worker {
  return new PierreDiffsWorker();
}

// Vite's `?worker` transform only yields a real Worker constructor in an
// actual browser/Vite build; under Vitest (vite-node + jsdom) the import
// resolves but isn't callable, which crashes WorkerPoolContextProvider at
// mount. Gate on it explicitly rather than assuming -- also protects any
// other runtime (SSR, a restrictive embedded webview) where module workers
// aren't available, falling back to MultiFileDiff's own `disableWorkerPool`
// (main-thread highlighting) instead of hard-crashing the diff view.
export const DIFFS_WORKER_SUPPORTED = typeof PierreDiffsWorker === "function";

// Token colors lifted from src/terminal/theme.ts's ANSI palette so diff
// syntax highlighting reads as the same "voice" as xterm output elsewhere in
// grove, instead of an off-the-shelf bundled theme with no relation to the
// rest of the app. Dark is grove's primary, designed-for surface; light
// reuses the same hue families darkened/saturated for contrast on white
// (grove's own light palette has no ANSI reference to port from -- see
// terminal/theme.ts's "dark values only" note).
const groveDarkTheme: ThemeRegistration = {
  name: "grove-dark",
  type: "dark",
  colors: {
    "editor.background": "#12151b",
    "editor.foreground": "#e7e9f0",
  },
  tokenColors: [
    { scope: ["comment", "punctuation.definition.comment"], settings: { foreground: "#5c6377", fontStyle: "italic" } },
    { scope: ["string", "string.quoted", "string.template"], settings: { foreground: "#2dd4bf" } },
    {
      scope: ["constant.numeric", "constant.language", "constant.character.escape", "constant.other"],
      settings: { foreground: "#f5a623" },
    },
    {
      scope: ["keyword", "keyword.control", "keyword.operator.new", "storage.modifier", "keyword.other"],
      settings: { foreground: "#c792ea" },
    },
    {
      scope: ["storage.type", "storage.type.function", "keyword.control.import", "keyword.control.export"],
      settings: { foreground: "#4c8dff" },
    },
    { scope: ["entity.name.function", "support.function", "meta.function-call"], settings: { foreground: "#5fd4c8" } },
    {
      scope: ["entity.name.class", "entity.name.type", "support.class", "support.type"],
      settings: { foreground: "#7aabff" },
    },
    { scope: ["entity.name.tag", "entity.other.attribute-name"], settings: { foreground: "#f0506e" } },
    { scope: ["variable.parameter"], settings: { foreground: "#969cb3" } },
    { scope: ["punctuation", "meta.brace"], settings: { foreground: "#969cb3" } },
  ],
};

const groveLightTheme: ThemeRegistration = {
  name: "grove-light",
  type: "light",
  colors: {
    "editor.background": "#ffffff",
    "editor.foreground": "#14161c",
  },
  tokenColors: [
    { scope: ["comment", "punctuation.definition.comment"], settings: { foreground: "#8a8fa0", fontStyle: "italic" } },
    { scope: ["string", "string.quoted", "string.template"], settings: { foreground: "#0f9488" } },
    {
      scope: ["constant.numeric", "constant.language", "constant.character.escape", "constant.other"],
      settings: { foreground: "#c2760f" },
    },
    {
      scope: ["keyword", "keyword.control", "keyword.operator.new", "storage.modifier", "keyword.other"],
      settings: { foreground: "#9333ea" },
    },
    {
      scope: ["storage.type", "storage.type.function", "keyword.control.import", "keyword.control.export"],
      settings: { foreground: "#2563eb" },
    },
    { scope: ["entity.name.function", "support.function", "meta.function-call"], settings: { foreground: "#0e7490" } },
    {
      scope: ["entity.name.class", "entity.name.type", "support.class", "support.type"],
      settings: { foreground: "#2563eb" },
    },
    { scope: ["entity.name.tag", "entity.other.attribute-name"], settings: { foreground: "#dc2626" } },
    { scope: ["variable.parameter"], settings: { foreground: "#5b6072" } },
    { scope: ["punctuation", "meta.brace"], settings: { foreground: "#5b6072" } },
  ],
};

registerCustomTheme("grove-dark", async () => groveDarkTheme);
registerCustomTheme("grove-light", async () => groveLightTheme);

/** Passed as FileDiffOptions.theme with themeType: "system" -- @pierre/diffs
 *  resolves both and switches between them via CSS's light-dark(), the same
 *  OS-preference-driven mechanism src/index.css uses (no manual toggle). */
export const GROVE_DIFF_THEME: ThemesType = { dark: "grove-dark", light: "grove-light" };
