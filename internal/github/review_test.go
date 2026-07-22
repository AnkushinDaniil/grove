package github

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestBuildReviewBody(t *testing.T) {
	in := SubmitInput{
		PR:    12540,
		Event: "REQUEST_CHANGES",
		Body:  "overall",
		Comments: []ReviewComment{
			{Path: "src/main.go", Line: 42, Side: "RIGHT", Body: "nit"},
		},
	}
	raw, err := buildReviewBody(in)
	if err != nil {
		t.Fatalf("buildReviewBody: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if got["event"] != "REQUEST_CHANGES" || got["body"] != "overall" {
		t.Errorf("body = %v, want event/body populated", got)
	}
	comments, ok := got["comments"].([]any)
	if !ok || len(comments) != 1 {
		t.Fatalf("comments = %v, want one element array", got["comments"])
	}
	c := comments[0].(map[string]any)
	if c["path"] != "src/main.go" || c["side"] != "RIGHT" || c["body"] != "nit" {
		t.Errorf("comment = %v, want path/side/body", c)
	}
	if c["line"].(float64) != 42 {
		t.Errorf("comment line = %v, want 42", c["line"])
	}
}

func TestBuildReviewBodyOmitsEmptyComments(t *testing.T) {
	raw, err := buildReviewBody(SubmitInput{Event: "APPROVE", Body: "lgtm"})
	if err != nil {
		t.Fatalf("buildReviewBody: %v", err)
	}
	if strings.Contains(string(raw), "comments") {
		t.Errorf("body = %s, want no comments key when empty", raw)
	}
}

func TestSubmitReviewArgsAndBody(t *testing.T) {
	var reviewArgs []string
	var sentBody []byte
	c := New(WithRunner(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[0] == "repo" {
			return []byte("octo/repo\n"), nil
		}
		reviewArgs = args
		// The body is handed to gh via --input <tempfile>; read it back.
		for i, a := range args {
			if a == "--input" && i+1 < len(args) {
				b, err := os.ReadFile(args[i+1])
				if err != nil {
					t.Fatalf("read input file: %v", err)
				}
				sentBody = b
			}
		}
		return []byte(`{"html_url":"https://github.com/octo/repo/pull/7#pullrequestreview-1"}`), nil
	}))

	url, err := c.SubmitReview(context.Background(), "/repo", SubmitInput{
		PR:       7,
		Event:    "COMMENT",
		Body:     "notes",
		Comments: []ReviewComment{{Path: "a.go", Line: 3, Side: "RIGHT", Body: "x"}},
	})
	if err != nil {
		t.Fatalf("SubmitReview() error = %v", err)
	}
	if url != "https://github.com/octo/repo/pull/7#pullrequestreview-1" {
		t.Errorf("url = %q, want the review html_url", url)
	}

	// Endpoint and method are built from the resolved repo name and PR number.
	joined := strings.Join(reviewArgs, " ")
	if !strings.Contains(joined, "repos/octo/repo/pulls/7/reviews") {
		t.Errorf("args = %v, want the reviews endpoint", reviewArgs)
	}
	if !strings.Contains(joined, "--method POST") || !strings.Contains(joined, "--input") {
		t.Errorf("args = %v, want POST with --input", reviewArgs)
	}

	// The temp file body carries the structured comments array intact.
	var body map[string]any
	if err := json.Unmarshal(sentBody, &body); err != nil {
		t.Fatalf("unmarshal sent body %q: %v", sentBody, err)
	}
	if body["event"] != "COMMENT" || len(body["comments"].([]any)) != 1 {
		t.Errorf("sent body = %v, want event COMMENT with one comment", body)
	}
}

func TestReplyToThreadNoResolve(t *testing.T) {
	var calls [][]string
	c := New(WithRunner(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		calls = append(calls, args)
		return []byte(`{"data":{}}`), nil
	}))
	if err := c.ReplyToThread(context.Background(), "/repo", ReplyInput{
		ThreadID: "PRRT_9", Body: "thanks", Resolve: false,
	}); err != nil {
		t.Fatalf("ReplyToThread() error = %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("gh calls = %d, want 1 (reply only)", len(calls))
	}
	want := []string{"api", "graphql", "-f", "query=" + addReplyMutation, "-f", "threadId=PRRT_9", "-f", "body=thanks"}
	if !reflect.DeepEqual(calls[0], want) {
		t.Errorf("reply args =\n %v\nwant\n %v", calls[0], want)
	}
}

func TestReplyToThreadWithResolve(t *testing.T) {
	var calls [][]string
	c := New(WithRunner(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		calls = append(calls, args)
		return []byte(`{"data":{}}`), nil
	}))
	if err := c.ReplyToThread(context.Background(), "/repo", ReplyInput{
		ThreadID: "PRRT_9", Body: "done", Resolve: true,
	}); err != nil {
		t.Fatalf("ReplyToThread() error = %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("gh calls = %d, want 2 (reply + resolve)", len(calls))
	}
	if !strings.Contains(strings.Join(calls[1], " "), "resolveReviewThread") {
		t.Errorf("second call = %v, want the resolve mutation", calls[1])
	}
	if !strings.Contains(strings.Join(calls[1], " "), "threadId=PRRT_9") {
		t.Errorf("second call = %v, want threadId passed", calls[1])
	}
}

func TestSubmitReviewBadResponse(t *testing.T) {
	c := New(WithRunner(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[0] == "repo" {
			return []byte("o/r"), nil
		}
		return []byte("not json"), nil // review endpoint returns garbage
	}))
	if _, err := c.SubmitReview(context.Background(), "/repo", SubmitInput{PR: 1, Event: "COMMENT"}); err == nil {
		t.Fatal("SubmitReview() error = nil, want a response parse error")
	}
}

func TestReplyToThreadReplyError(t *testing.T) {
	c := New(WithRunner(func(context.Context, string, ...string) ([]byte, error) {
		return nil, &GHError{Args: []string{"api", "graphql"}, ExitCode: 1, Stderr: "boom"}
	}))
	if err := c.ReplyToThread(context.Background(), "/repo", ReplyInput{ThreadID: "T", Body: "x"}); err == nil {
		t.Fatal("ReplyToThread() error = nil, want the reply mutation error")
	}
}

func TestReplyToThreadResolveError(t *testing.T) {
	var calls int
	c := New(WithRunner(func(context.Context, string, ...string) ([]byte, error) {
		calls++
		if calls == 1 {
			return []byte(`{"data":{}}`), nil // reply succeeds
		}
		return nil, &GHError{Args: []string{"api", "graphql"}, ExitCode: 1, Stderr: "boom"}
	}))
	if err := c.ReplyToThread(context.Background(), "/repo", ReplyInput{ThreadID: "T", Body: "x", Resolve: true}); err == nil {
		t.Fatal("ReplyToThread() error = nil, want the resolve mutation error")
	}
}

func TestSubmitReviewRepoNameError(t *testing.T) {
	c := New(WithRunner(func(context.Context, string, ...string) ([]byte, error) {
		return nil, &GHError{Args: []string{"repo", "view"}, ExitCode: 1, Stderr: "no remote"}
	}))
	if _, err := c.SubmitReview(context.Background(), "/repo", SubmitInput{PR: 1, Event: "COMMENT"}); err == nil {
		t.Fatal("SubmitReview() error = nil, want error when repo name fails")
	}
}
