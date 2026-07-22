import { describe, expect, it } from "vitest";
import { hasSuggestion, joinSuggestion, splitSuggestion } from "./suggestion";

describe("joinSuggestion", () => {
  it("appends a suggestion block after prose", () => {
    expect(joinSuggestion("Wrap this call.", "const x = safe()")).toBe(
      "Wrap this call.\n\n```suggestion\nconst x = safe()\n```",
    );
  });

  it("emits a bare block when there is no prose", () => {
    expect(joinSuggestion("", "x := 1")).toBe("```suggestion\nx := 1\n```");
  });

  it("drops a blank suggestion, keeping a comment-only body", () => {
    expect(joinSuggestion("Just a note.", "")).toBe("Just a note.");
    expect(joinSuggestion("Just a note.", "   \n")).toBe("Just a note.");
  });
});

describe("splitSuggestion", () => {
  it("returns the whole body as text when there is no block", () => {
    expect(splitSuggestion("plain comment")).toEqual({ text: "plain comment", suggestion: "" });
  });

  it("extracts the suggestion and the surrounding prose", () => {
    const body = "Prose.\n\n```suggestion\nnew line\n```";
    expect(splitSuggestion(body)).toEqual({ text: "Prose.", suggestion: "new line" });
  });

  it("round-trips with joinSuggestion", () => {
    const text = "Consider early return.";
    const suggestion = "if err != nil { return err }";
    expect(splitSuggestion(joinSuggestion(text, suggestion))).toEqual({ text, suggestion });
  });
});

describe("hasSuggestion", () => {
  it("detects a suggestion block", () => {
    expect(hasSuggestion("a\n\n```suggestion\nb\n```")).toBe(true);
    expect(hasSuggestion("no block here")).toBe(false);
  });
});
