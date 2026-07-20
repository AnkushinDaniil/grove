import type { ITheme } from "@xterm/xterm";

// xterm's Terminal constructor needs literal colors, not CSS custom
// properties -- xterm doesn't resolve `var(...)`. Keep in sync with the
// --color-* tokens in src/index.css if the palette changes; dark values
// only, since the terminal renders actual process output where forcing a
// light background would fight the process's own ANSI choices.
export const terminalTheme: ITheme = {
  background: "#0a0c10",
  foreground: "#e7e9f0",
  cursor: "#a3e635",
  cursorAccent: "#0a0c10",
  selectionBackground: "rgba(163, 230, 53, 0.25)",

  black: "#12151b",
  red: "#f0506e",
  green: "#2dd4bf",
  yellow: "#f5a623",
  blue: "#4c8dff",
  magenta: "#c792ea",
  cyan: "#5fd4c8",
  white: "#c7cbd6",

  brightBlack: "#5c6377",
  brightRed: "#ff6b85",
  brightGreen: "#5eead4",
  brightYellow: "#ffc266",
  brightBlue: "#7aabff",
  brightMagenta: "#dcb8f5",
  brightCyan: "#8fe9de",
  brightWhite: "#e7e9f0",
};
