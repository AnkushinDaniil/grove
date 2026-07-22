package github

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// PRReview is grove's internal, fully-assembled view of one pull request for the
// interactive review workspace: metadata, the per-file diff, and the existing
// review threads. The wire shape (docs/API.md "Interactive review workspace") is
// a snake_case projection of this, mapped in internal/api.
type PRReview struct {
	Number         int
	Title          string
	Author         string
	URL            string
	State          string // OPEN|CLOSED|MERGED
	HeadSHA        string
	BaseRef        string
	Checks         string // passing|failing|pending|none
	ReviewDecision string
	Body           string
	Files          []PRFile
	Threads        []Thread
}

// PRFile is one changed file's diff. Binary (or too-large) files carry no patch
// from GitHub, so Binary is true and Hunks is empty.
type PRFile struct {
	Path      string
	OldPath   string // previous_filename, set for renames
	Status    string // modified|added|removed|renamed
	Additions int
	Deletions int
	Binary    bool
	Hunks     []Hunk
}

// Thread is one review-comment thread anchored to a file line. Line is 0 for an
// outdated thread whose anchor GitHub can no longer resolve.
type Thread struct {
	ID         string
	Path       string
	Line       int
	Side       string // RIGHT|LEFT
	IsResolved bool
	DiffHunk   string
	Comments   []ThreadComment
}

// ThreadComment is one comment within a Thread. IsMine is true when the author
// is the authenticated gh user.
type ThreadComment struct {
	ID        string
	Author    string
	Body      string
	CreatedAt string
	IsMine    bool
}

// reviewThreadsQuery fetches a PR's review threads with their comments. Line may
// be null for outdated threads; diffSide is RIGHT/LEFT.
const reviewThreadsQuery = `query($owner:String!,$repo:String!,$pr:Int!){
  repository(owner:$owner,name:$repo){
    pullRequest(number:$pr){
      reviewThreads(first:100){
        nodes{
          id
          isResolved
          line
          path
          diffSide
          comments(first:50){
            nodes{ id author{login} body createdAt diffHunk }
          }
        }
      }
    }
  }
}`

// PRDetail assembles the full review view of pull request pr in the repository
// rooted at dir: metadata via `gh pr view`, the per-file diff via the REST files
// endpoint, and existing review threads via GraphQL. Every gh call flows through
// the Client's runner seam.
func (c *Client) PRDetail(ctx context.Context, dir string, pr int) (PRReview, error) {
	name, err := c.RepoName(ctx, dir)
	if err != nil {
		return PRReview{}, err
	}

	meta, err := c.prMeta(ctx, dir, pr)
	if err != nil {
		return PRReview{}, err
	}
	files, err := c.prFiles(ctx, dir, name, pr)
	if err != nil {
		return PRReview{}, err
	}
	// Login is best-effort: without it is_mine simply stays false everywhere,
	// which is preferable to failing the whole read.
	login, _ := c.Login(ctx)
	threads, err := c.prThreads(ctx, dir, name, pr, login)
	if err != nil {
		return PRReview{}, err
	}

	review := meta
	review.Files = files
	review.Threads = threads
	return review, nil
}

// rawPRView mirrors the `gh pr view --json` object for a single PR.
type rawPRView struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
	URL               string             `json:"url"`
	State             string             `json:"state"`
	HeadRefOid        string             `json:"headRefOid"`
	BaseRefName       string             `json:"baseRefName"`
	ReviewDecision    string             `json:"reviewDecision"`
	Body              string             `json:"body"`
	StatusCheckRollup []checkRollupEntry `json:"statusCheckRollup"`
}

// prMeta reads the PR's metadata and folds its check rollup into a summary.
func (c *Client) prMeta(ctx context.Context, dir string, pr int) (PRReview, error) {
	const fields = "number,title,author,url,state,headRefOid,baseRefName,reviewDecision,body,statusCheckRollup"
	out, err := c.call(ctx, dir, "pr", "view", strconv.Itoa(pr), "--json", fields)
	if err != nil {
		return PRReview{}, fmt.Errorf("gh pr view: %w", err)
	}
	var raw rawPRView
	if err := json.Unmarshal(out, &raw); err != nil {
		return PRReview{}, fmt.Errorf("parse gh pr view output: %w", err)
	}
	return PRReview{
		Number:         raw.Number,
		Title:          raw.Title,
		Author:         raw.Author.Login,
		URL:            raw.URL,
		State:          raw.State,
		HeadSHA:        raw.HeadRefOid,
		BaseRef:        raw.BaseRefName,
		Checks:         summarizeChecks(raw.StatusCheckRollup),
		ReviewDecision: raw.ReviewDecision,
		Body:           raw.Body,
	}, nil
}

// rawFile mirrors one entry of the REST pulls/{pr}/files response.
type rawFile struct {
	Filename         string `json:"filename"`
	PreviousFilename string `json:"previous_filename"`
	Status           string `json:"status"`
	Additions        int    `json:"additions"`
	Deletions        int    `json:"deletions"`
	Patch            string `json:"patch"`
}

// prFiles reads the PR's changed files (paginated) and parses each file's patch
// into hunks.
func (c *Client) prFiles(ctx context.Context, dir, nameWithOwner string, pr int) ([]PRFile, error) {
	endpoint := fmt.Sprintf("repos/%s/pulls/%d/files", nameWithOwner, pr)
	out, err := c.call(ctx, dir, "api", endpoint, "--paginate")
	if err != nil {
		return nil, fmt.Errorf("gh api %s: %w", endpoint, err)
	}
	raw, err := decodeFilePages(out)
	if err != nil {
		return nil, err
	}
	files := make([]PRFile, 0, len(raw))
	for _, rf := range raw {
		files = append(files, rf.toPRFile())
	}
	return files, nil
}

// decodeFilePages decodes the files response, tolerating gh's --paginate output.
// For an array endpoint gh may emit either one merged array or several arrays
// back to back; a streaming decoder reads whichever shape and concatenates them.
func decodeFilePages(out []byte) ([]rawFile, error) {
	dec := json.NewDecoder(strings.NewReader(string(out)))
	var files []rawFile
	for dec.More() {
		var page []rawFile
		if err := dec.Decode(&page); err != nil {
			return nil, fmt.Errorf("parse gh files output: %w", err)
		}
		files = append(files, page...)
	}
	return files, nil
}

// toPRFile projects a raw files-endpoint entry into a PRFile. An empty patch
// (binary or too-large file) yields Binary=true and no hunks.
func (rf rawFile) toPRFile() PRFile {
	return PRFile{
		Path:      rf.Filename,
		OldPath:   rf.PreviousFilename,
		Status:    rf.Status,
		Additions: rf.Additions,
		Deletions: rf.Deletions,
		Binary:    rf.Patch == "",
		Hunks:     parsePatch(rf.Patch),
	}
}

// rawThreadsResponse mirrors the GraphQL reviewThreads response envelope.
type rawThreadsResponse struct {
	Data struct {
		Repository struct {
			PullRequest struct {
				ReviewThreads struct {
					Nodes []rawThread `json:"nodes"`
				} `json:"reviewThreads"`
			} `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
}

type rawThread struct {
	ID         string `json:"id"`
	IsResolved bool   `json:"isResolved"`
	Line       *int   `json:"line"`
	Path       string `json:"path"`
	DiffSide   string `json:"diffSide"`
	Comments   struct {
		Nodes []rawThreadComment `json:"nodes"`
	} `json:"comments"`
}

type rawThreadComment struct {
	ID     string `json:"id"`
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
	Body      string `json:"body"`
	CreatedAt string `json:"createdAt"`
	DiffHunk  string `json:"diffHunk"`
}

// prThreads reads the PR's review threads via GraphQL. login identifies the
// authenticated user for is_mine; an empty login leaves every comment not-mine.
func (c *Client) prThreads(ctx context.Context, dir, nameWithOwner string, pr int, login string) ([]Thread, error) {
	owner, repo, ok := strings.Cut(nameWithOwner, "/")
	if !ok {
		return nil, fmt.Errorf("unexpected repo name %q, want owner/repo", nameWithOwner)
	}
	out, err := c.call(ctx, dir, "api", "graphql",
		"-f", "query="+reviewThreadsQuery,
		"-f", "owner="+owner,
		"-f", "repo="+repo,
		"-F", "pr="+strconv.Itoa(pr),
	)
	if err != nil {
		return nil, fmt.Errorf("gh api graphql reviewThreads: %w", err)
	}
	var resp rawThreadsResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("parse gh graphql reviewThreads output: %w", err)
	}

	nodes := resp.Data.Repository.PullRequest.ReviewThreads.Nodes
	threads := make([]Thread, 0, len(nodes))
	for _, rt := range nodes {
		threads = append(threads, rt.toThread(login))
	}
	return threads, nil
}

// toThread projects a raw GraphQL thread into a Thread. A null line becomes 0
// (outdated thread) and the thread's diff hunk is taken from its first comment.
func (rt rawThread) toThread(login string) Thread {
	line := 0
	if rt.Line != nil {
		line = *rt.Line
	}
	comments := make([]ThreadComment, 0, len(rt.Comments.Nodes))
	diffHunk := ""
	for i, rc := range rt.Comments.Nodes {
		if i == 0 {
			diffHunk = rc.DiffHunk
		}
		comments = append(comments, ThreadComment{
			ID:        rc.ID,
			Author:    rc.Author.Login,
			Body:      rc.Body,
			CreatedAt: rc.CreatedAt,
			IsMine:    login != "" && rc.Author.Login == login,
		})
	}
	return Thread{
		ID:         rt.ID,
		Path:       rt.Path,
		Line:       line,
		Side:       rt.DiffSide,
		IsResolved: rt.IsResolved,
		DiffHunk:   diffHunk,
		Comments:   comments,
	}
}
