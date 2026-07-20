// Token handoff per docs/API.md: the daemon hands the browser a one-time
// token in the URL fragment (`#t=...`); the UI exchanges it for an HttpOnly
// session cookie via POST /auth/session, then scrubs the fragment so the
// token never lingers in browser history.

export const CSRF_HEADER = "X-Grove-CSRF";

export function extractHashToken(hash: string): string | null {
  if (!hash || hash.length < 2) return null;
  const params = new URLSearchParams(hash.slice(1));
  return params.get("t");
}

export function stripHashToken(): void {
  const url = new URL(window.location.href);
  url.hash = "";
  window.history.replaceState(null, "", url.pathname + url.search);
}
