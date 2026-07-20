import type { TermControlMessage } from "../gen/types";
import type { TermTransport } from "../terminal/termTypes";

const REPLAY_LINES = [
  "\x1b[2mgrove\x1b[0m \x1b[1;32m❯\x1b[0m claude --resume claude-sess-3f9a7c\r\n",
  "\x1b[2m── resuming session ──\x1b[0m\r\n",
  "\x1b[1mWiring the fit addon to a ResizeObserver…\x1b[0m\r\n",
];

const LIVE_LINES = [
  "Reading src/terminal/XtermHost.tsx\r\n",
  "Editing src/terminal/XtermHost.tsx\r\n",
  "  \x1b[32m+\x1b[0m WebGL addon now loads only while this pane is focused\r\n",
  "Running npm run test…\r\n",
  "\x1b[32m PASS \x1b[0m src/state/ws.test.ts\r\n",
  "\x1b[32m PASS \x1b[0m src/state/tree.test.ts\r\n",
];

/** Scripted /ws/term stand-in: replays a few banner lines, flips to "live",
 *  then trickles in canned agent output plus a local echo of typed input --
 *  enough to exercise XtermHost's full attach lifecycle with zero backend. */
export class MockTermTransport implements TermTransport {
  private onBinary: ((data: Uint8Array) => void) | null = null;
  private onControl: ((msg: TermControlMessage) => void) | null = null;
  private liveTimer: ReturnType<typeof setInterval> | null = null;
  private timeouts: ReturnType<typeof setTimeout>[] = [];
  private stopped = false;
  private lineIndex = 0;

  start(
    _cols: number,
    _rows: number,
    onBinary: (data: Uint8Array) => void,
    onControl: (msg: TermControlMessage) => void,
  ): void {
    this.onBinary = onBinary;
    this.onControl = onControl;
    const encoder = new TextEncoder();

    let i = 0;
    const replayNext = () => {
      if (this.stopped) return;
      if (i >= REPLAY_LINES.length) {
        this.onControl?.({ t: "live" });
        this.liveTimer = setInterval(() => this.emitLive(encoder), 2600);
        return;
      }
      this.onBinary?.(encoder.encode(REPLAY_LINES[i]));
      i += 1;
      this.timeouts.push(setTimeout(replayNext, 90));
    };
    this.timeouts.push(setTimeout(replayNext, 150));
  }

  resize(): void {
    // No-op: nothing on the mock "server" cares about pty dimensions.
  }

  sendInput(data: Uint8Array): void {
    if (this.stopped) return;
    // Local echo so typed characters are visible, like a real shell.
    this.onBinary?.(data);
    if (new TextDecoder().decode(data) === "\r") {
      this.onBinary?.(new TextEncoder().encode("\r\n"));
    }
  }

  stop(): void {
    this.stopped = true;
    if (this.liveTimer !== null) clearInterval(this.liveTimer);
    for (const t of this.timeouts) clearTimeout(t);
    this.timeouts = [];
    this.onBinary = null;
    this.onControl = null;
  }

  private emitLive(encoder: TextEncoder): void {
    if (this.stopped) return;
    const line = LIVE_LINES[this.lineIndex % LIVE_LINES.length];
    this.lineIndex += 1;
    this.onBinary?.(encoder.encode(line));
  }
}

export function createMockTermTransport(): TermTransport {
  return new MockTermTransport();
}
