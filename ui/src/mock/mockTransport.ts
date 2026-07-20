import type { StateSocketHandlers, StateTransport } from "../state/ws";
import { world } from "./world";
import { SESSION_TERM_ID, TASK_UI_TERM_ID } from "./fixtures";

const AMBIENT_TICK_MS = 5000;
const HELLO_DELAY_MS = 120;

const AMBIENT_LINES = [
  "Checking ResizeObserver wiring against the fit addon…",
  "Running `npm run test` for the ws client…",
  "Reviewing WebGL addon dispose-on-blur behavior…",
  "Cross-checking status dot pulse timing against the design brief…",
];

/** Scripted /ws/state stand-in: sends an initial hello from the shared
 *  world snapshot, forwards every subsequent world.publish() (i.e. every
 *  mock REST mutation) as a delta, and injects a low-frequency ambient tick
 *  so the demo feels alive even with no user interaction. */
export class MockStateTransport implements StateTransport {
  private readonly handlers: StateSocketHandlers;
  private unsubscribe: (() => void) | null = null;
  private ambientTimer: ReturnType<typeof setInterval> | null = null;
  private helloTimer: ReturnType<typeof setTimeout> | null = null;
  private tickIndex = 0;

  constructor(handlers: StateSocketHandlers) {
    this.handlers = handlers;
  }

  connect(): void {
    this.handlers.onStatusChange("connecting");
    this.helloTimer = setTimeout(() => {
      this.helloTimer = null;
      const snap = world.snapshot();
      this.handlers.onHello(snap.rev, snap.nodes, snap.sessions, world.inbox());
      this.handlers.onStatusChange("open");

      this.unsubscribe = world.subscribe((rev, patch) => {
        this.handlers.onDelta(rev, patch.nodes, patch.sessions, patch.events);
      });
      this.ambientTimer = setInterval(() => this.tick(), AMBIENT_TICK_MS);
    }, HELLO_DELAY_MS);
  }

  stop(): void {
    if (this.helloTimer !== null) clearTimeout(this.helloTimer);
    if (this.ambientTimer !== null) clearInterval(this.ambientTimer);
    this.helloTimer = null;
    this.ambientTimer = null;
    this.unsubscribe?.();
    this.unsubscribe = null;
    this.handlers.onStatusChange("closed");
  }

  private tick(): void {
    const node = world.nodesById.get(TASK_UI_TERM_ID);
    if (!node || node.status !== "running") return;
    const text = AMBIENT_LINES[this.tickIndex % AMBIENT_LINES.length];
    this.tickIndex += 1;
    world.publish({
      events: [
        {
          id: world.nextId("evt"),
          node_id: TASK_UI_TERM_ID,
          session_id: SESSION_TERM_ID,
          type: "text",
          payload: { text, final: false },
          requires_attention: false,
          created_at: new Date().toISOString(),
        },
      ],
    });
  }
}
