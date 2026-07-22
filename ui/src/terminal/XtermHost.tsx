import { useEffect, useRef, useState } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { SearchAddon } from "@xterm/addon-search";
import { Unicode11Addon } from "@xterm/addon-unicode11";
import type { WebglAddon } from "@xterm/addon-webgl";
import "@xterm/xterm/css/xterm.css";
import clsx from "clsx";
import { terminalTheme } from "./theme";
import { useTermSocket } from "./useTermSocket";
import { useTerminalPoolStore } from "../state/terminalPool";
import { TerminalSearchBar } from "./TerminalSearchBar";

const TERMINAL_FONT_FAMILY =
  '"JetBrains Mono", ui-monospace, "SF Mono", "Cascadia Mono", "Cascadia Code", Menlo, Consolas, "Liberation Mono", monospace';

/** Touch/coarse-pointer screens are typically viewed at higher effective
 *  pixel density than a desktop monitor -- bump legibility up from the
 *  13px desktop default rather than just clearing a 12px floor. */
function preferredFontSize(): number {
  if (typeof window === "undefined" || !window.matchMedia) return 13;
  return window.matchMedia("(pointer: coarse)").matches ? 14 : 13;
}

interface XtermHostProps {
  sessionId: string;
  className?: string;
}

/**
 * Attaches to /ws/term/{sessionId}: opens xterm, streams replay then live
 * bytes, and tears down cleanly. Callers should render this with
 * `key={sessionId}` so a session change gets a fully fresh instance rather
 * than trying to rebind state onto a reused one.
 */
export function XtermHost({ sessionId, className }: XtermHostProps) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const termRef = useRef<Terminal | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const searchAddonRef = useRef<SearchAddon | null>(null);
  const webglAddonRef = useRef<WebglAddon | null>(null);

  const [isLive, setIsLive] = useState(false);
  const [exitCode, setExitCode] = useState<number | null>(null);
  const [searchOpen, setSearchOpen] = useState(false);
  const [hasSearchAddon, setHasSearchAddon] = useState(false);

  const touch = useTerminalPoolStore((s) => s.touch);

  useEffect(() => {
    touch(sessionId);
  }, [sessionId, touch]);

  // Create the terminal instance. Registered before useTermSocket below so
  // this effect commits first, guaranteeing termRef is populated by the
  // time useTermSocket's own effect reads the initial size.
  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const term = new Terminal({
      fontFamily: TERMINAL_FONT_FAMILY,
      fontSize: preferredFontSize(),
      lineHeight: 1.35,
      theme: terminalTheme,
      cursorBlink: true,
      cursorStyle: "bar",
      scrollback: 10_000,
      allowProposedApi: true,
    });
    const fitAddon = new FitAddon();
    const searchAddon = new SearchAddon();
    const unicode11Addon = new Unicode11Addon();

    term.loadAddon(fitAddon);
    term.loadAddon(searchAddon);
    term.loadAddon(unicode11Addon);
    term.unicode.activeVersion = "11";

    term.open(container);
    fitAddon.fit();
    term.focus();

    termRef.current = term;
    fitAddonRef.current = fitAddon;
    searchAddonRef.current = searchAddon;
    setHasSearchAddon(true);

    // Upgrade to the WebGL renderer once, at mount. An earlier version toggled
    // it on textarea focus/blur to conserve GPU contexts, but loading the
    // addon rebuilds the render layer and was intermittently stealing keyboard
    // focus the moment the user clicked in -- leaving the terminal unfocusable
    // and unable to receive input. Only a handful of terminals are ever mounted
    // at once (one per visible node view), far under the browser's context cap,
    // so a persistent context per terminal is fine.
    void import("@xterm/addon-webgl").then(({ WebglAddon: WebglAddonCtor }) => {
      if (termRef.current !== term) return; // torn down while the import was in flight
      try {
        const webgl = new WebglAddonCtor();
        term.loadAddon(webgl);
        webglAddonRef.current = webgl;
      } catch {
        // WebGL unavailable (disabled GPU, headless CI, ...) -- the default
        // renderer already works, so just skip the upgrade.
      }
    });

    return () => {
      webglAddonRef.current?.dispose();
      webglAddonRef.current = null;
      term.dispose();
      termRef.current = null;
      fitAddonRef.current = null;
      searchAddonRef.current = null;
      setHasSearchAddon(false);
    };
  }, [sessionId]);

  // Focus the terminal as soon as its session goes live, so a freshly attached
  // session is immediately typeable without a click.
  useEffect(() => {
    if (isLive) termRef.current?.focus();
  }, [isLive]);

  const { resize, sendInput } = useTermSocket({
    sessionId,
    enabled: exitCode === null,
    getInitialSize: () => {
      const term = termRef.current;
      return { cols: term?.cols ?? 80, rows: term?.rows ?? 24 };
    },
    onData: (data) => termRef.current?.write(data),
    onLive: () => setIsLive(true),
    onExit: (code) => setExitCode(code),
  });

  // Keystrokes -> binary frames.
  useEffect(() => {
    const term = termRef.current;
    if (!term) return;
    const disposable = term.onData((data) => {
      sendInput(new TextEncoder().encode(data));
    });
    return () => disposable.dispose();
  }, [sendInput, isLive]);

  // Keep the pty size in sync with the container.
  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const observer = new ResizeObserver(() => {
      const term = termRef.current;
      const fitAddon = fitAddonRef.current;
      if (!term || !fitAddon) return;
      const prevCols = term.cols;
      const prevRows = term.rows;
      fitAddon.fit();
      if (term.cols !== prevCols || term.rows !== prevRows) {
        resize(term.cols, term.rows);
      }
    });
    observer.observe(container);
    return () => observer.disconnect();
  }, [resize]);

  // Cmd/Ctrl+F opens xterm's own search instead of the browser's
  // find-in-page, but only while this terminal actually has focus.
  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      const el = termRef.current?.element;
      if (!el || !el.contains(document.activeElement)) return;
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "f") {
        e.preventDefault();
        setSearchOpen(true);
      } else if (e.key === "Escape" && searchOpen) {
        setSearchOpen(false);
        termRef.current?.focus();
      }
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [searchOpen]);

  return (
    // Any mousedown in the terminal area (including the padding gutter) focuses
    // xterm's input, so clicking anywhere lets the user type immediately.
    <div
      className={clsx("relative flex h-full min-h-0 flex-col bg-canvas", className)}
      onMouseDown={() => termRef.current?.focus()}
    >
      {!isLive && exitCode === null && (
        <div className="pointer-events-none absolute inset-x-0 top-0 z-10 flex items-center gap-2 border-b border-border bg-surface-2/90 px-3 py-1.5 text-2xs text-ink-faint backdrop-blur-sm">
          <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-ink-faint" />
          syncing scrollback…
        </div>
      )}
      {/* overscroll-contain keeps terminal wheel/touch scrolling from chaining
          to the page once the scrollback hits its top or bottom. */}
      <div ref={containerRef} className="min-h-0 flex-1 overflow-hidden overscroll-contain px-3 py-2" />
      {hasSearchAddon && searchOpen && (
        <TerminalSearchBar
          onClose={() => {
            setSearchOpen(false);
            termRef.current?.focus();
          }}
          onNext={(q) => searchAddonRef.current?.findNext(q)}
          onPrev={(q) => searchAddonRef.current?.findPrevious(q)}
        />
      )}
      {exitCode !== null && (
        <div className="absolute inset-x-0 bottom-0 z-10 flex items-center gap-2 border-t border-border-strong bg-surface-2 px-3 py-2 text-xs">
          <span
            className={clsx(
              "h-1.5 w-1.5 rounded-full",
              exitCode === 0 ? "bg-status-done" : "bg-status-failed",
            )}
          />
          <span className="text-ink">Process exited</span>
          <span className="text-ink-faint">code {exitCode}</span>
        </div>
      )}
    </div>
  );
}
