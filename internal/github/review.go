package github

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

// ReviewComment is one inline comment posted as part of a batch review. Line is
// the line number on the diff Side (RIGHT for the new file, LEFT for the old).
type ReviewComment struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Side string `json:"side"`
	Body string `json:"body"`
}

// SubmitInput is a batch pull-request review: an overall verdict plus optional
// inline comments.
type SubmitInput struct {
	PR       int
	Event    string // APPROVE|REQUEST_CHANGES|COMMENT
	Body     string
	Comments []ReviewComment
}

// reviewBody is the JSON body of the GitHub create-review REST call.
type reviewBody struct {
	Event    string          `json:"event"`
	Body     string          `json:"body"`
	Comments []ReviewComment `json:"comments,omitempty"`
}

// buildReviewBody builds the create-review request body. Comments are omitted
// when empty (an approve/comment with no inline notes), matching the GitHub API.
func buildReviewBody(in SubmitInput) ([]byte, error) {
	body, err := json.Marshal(reviewBody{
		Event:    in.Event,
		Body:     in.Body,
		Comments: in.Comments,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal review body: %w", err)
	}
	return body, nil
}

// SubmitReview posts a single batch review for pull request in.PR in the
// repository rooted at dir and returns the created review's html_url. The
// request body is written to a temp file and passed via `gh api --input`, so the
// structured comments array survives the CLI intact (gh field flags cannot build
// an array of objects) while every call still flows through the runner seam.
func (c *Client) SubmitReview(ctx context.Context, dir string, in SubmitInput) (string, error) {
	name, err := c.RepoName(ctx, dir)
	if err != nil {
		return "", err
	}
	body, err := buildReviewBody(in)
	if err != nil {
		return "", err
	}

	f, err := os.CreateTemp("", "grove-review-*.json")
	if err != nil {
		return "", fmt.Errorf("create review body file: %w", err)
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err := f.Write(body); err != nil {
		_ = f.Close()
		return "", fmt.Errorf("write review body: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("close review body: %w", err)
	}

	endpoint := fmt.Sprintf("repos/%s/pulls/%d/reviews", name, in.PR)
	out, err := c.call(ctx, dir, "api", endpoint, "--method", "POST", "--input", f.Name())
	if err != nil {
		return "", fmt.Errorf("gh api %s: %w", endpoint, err)
	}
	var resp struct {
		HTMLURL string `json:"html_url"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", fmt.Errorf("parse gh review response: %w", err)
	}
	return resp.HTMLURL, nil
}

// ReplyInput is a reply to an existing review thread, optionally resolving it.
type ReplyInput struct {
	ThreadID string
	Body     string
	Resolve  bool
}

// addReplyMutation appends a reply comment to an existing review thread.
const addReplyMutation = `mutation($threadId:ID!,$body:String!){
  addPullRequestReviewThreadReply(input:{pullRequestReviewThreadId:$threadId,body:$body}){
    comment{ id }
  }
}`

// resolveThreadMutation marks a review thread resolved.
const resolveThreadMutation = `mutation($threadId:ID!){
  resolveReviewThread(input:{threadId:$threadId}){ thread{ id } }
}`

// ReplyToThread posts a reply to review thread in.ThreadID and, when
// in.Resolve is set, resolves the thread. Both are GraphQL mutations through the
// runner seam.
func (c *Client) ReplyToThread(ctx context.Context, dir string, in ReplyInput) error {
	if _, err := c.call(ctx, dir, "api", "graphql",
		"-f", "query="+addReplyMutation,
		"-f", "threadId="+in.ThreadID,
		"-f", "body="+in.Body,
	); err != nil {
		return fmt.Errorf("gh api graphql addReply: %w", err)
	}
	if !in.Resolve {
		return nil
	}
	if _, err := c.call(ctx, dir, "api", "graphql",
		"-f", "query="+resolveThreadMutation,
		"-f", "threadId="+in.ThreadID,
	); err != nil {
		return fmt.Errorf("gh api graphql resolveThread: %w", err)
	}
	return nil
}
