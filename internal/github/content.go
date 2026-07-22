package github

import "bytes"

// MaxContentBytes caps how large either side of a file may be before its full
// contents are omitted from a review (docs/API.md "Diff content for rich
// rendering"): ~512 KB. Larger files fall back to the "view on GitHub / open in
// editor" placeholder the UI renders when content_omitted is set.
const MaxContentBytes = 512 * 1024

// ContentDecision is the resolved rich-diff payload for one file: the two sides'
// full text and the omission reason. Omitted is "" (show both sides), "binary",
// or "too_large"; when it is non-empty, Original and Modified are empty.
type ContentDecision struct {
	Original string
	Modified string
	Omitted  string
}

// DecideContent applies the shared size/binary rules that both PR review (Part
// 1) and worktree review (Part 2) use to turn a file's two raw sides into a
// ContentDecision. forceBinary short-circuits to "binary" when the source
// already knows the file is not text (GitHub carried no patch, or a base64
// decode failed). Otherwise a side over MaxContentBytes yields "too_large", a
// NUL byte in either side yields "binary", and a text file returns both sides
// verbatim. An added side (nil/empty original) or a removed side (nil/empty
// modified) is simply the empty string, per the contract.
func DecideContent(original, modified []byte, forceBinary bool) ContentDecision {
	if forceBinary {
		return ContentDecision{Omitted: "binary"}
	}
	if len(original) > MaxContentBytes || len(modified) > MaxContentBytes {
		return ContentDecision{Omitted: "too_large"}
	}
	if bytes.IndexByte(original, 0) >= 0 || bytes.IndexByte(modified, 0) >= 0 {
		return ContentDecision{Omitted: "binary"}
	}
	return ContentDecision{Original: string(original), Modified: string(modified)}
}
