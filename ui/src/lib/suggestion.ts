// GitHub renders a review comment as a committable "suggested change" when its
// body contains a ```suggestion fenced block whose content replaces the
// anchored line(s). grove keeps a finding's prose and its suggested code as two
// separate fields for editing, and (de)serializes them to that one body string
// at the store/GitHub boundary -- these helpers are that boundary.

// SUGGESTION_FENCE matches a ```suggestion block, capturing the replacement code
// between the fences. Non-greedy so a body with trailing prose after the block
// still splits at the first block.
const SUGGESTION_FENCE = /```suggestion[ \t]*\r?\n([\s\S]*?)\r?\n?```/;

/**
 * splitSuggestion separates a comment body into its prose and the suggested
 * replacement code (the ```suggestion block), if any. A body with no suggestion
 * block yields an empty suggestion.
 */
export function splitSuggestion(body: string): { text: string; suggestion: string } {
  const m = body.match(SUGGESTION_FENCE);
  if (!m || m.index === undefined) return { text: body, suggestion: "" };
  const text = (body.slice(0, m.index) + body.slice(m.index + m[0].length)).trim();
  return { text, suggestion: m[1] };
}

/**
 * joinSuggestion composes a comment body from prose and a suggested replacement,
 * appending the ```suggestion block GitHub needs. A blank suggestion produces a
 * comment-only body (we do not emit empty suggestions, which would delete the
 * line).
 */
export function joinSuggestion(text: string, suggestion: string): string {
  const code = suggestion.replace(/\s+$/, "");
  const prose = text.trim();
  if (code.trim() === "") return prose;
  const block = "```suggestion\n" + code + "\n```";
  return prose === "" ? block : prose + "\n\n" + block;
}

/** hasSuggestion reports whether a body carries a ```suggestion block. */
export function hasSuggestion(body: string): boolean {
  return SUGGESTION_FENCE.test(body);
}
