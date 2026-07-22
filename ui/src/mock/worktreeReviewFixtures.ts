import type { WorktreeComment, WorktreeReview } from "../gen/types";
import { ago, MIN, TASK_STRIPE_ID } from "./fixtures";

// The hero fixture for the worktree review tab -- reuses task-stripe (see
// mock/fixtures.ts), whose attention/attention_reason ("Worktree has 1 open
// PR comment thread") already describes exactly this scenario, so demoing
// the Review tab and the existing attention badge tell a consistent story.
export const WORKTREE_REVIEW_NODE_ID = TASK_STRIPE_ID;
export const WORKTREE_REPO = "billing-service";

const ORIGINAL_WEBHOOK_GO = `package billing

import (
	"encoding/json"
	"net/http"

	"github.com/daniil/billing-service/internal/invoice"
)

// HandleStripeWebhook processes incoming Stripe webhook events for invoice
// lifecycle changes.
func HandleStripeWebhook(w http.ResponseWriter, r *http.Request) {
	var evt StripeEvent
	if err := json.NewDecoder(r.Body).Decode(&evt); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	switch evt.Type {
	case "invoice.paid":
		if err := invoice.MarkPaid(evt.Data.InvoiceID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	case "invoice.payment_failed":
		if err := invoice.MarkFailed(evt.Data.InvoiceID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}
`;

const MODIFIED_WEBHOOK_GO = `package billing

import (
	"encoding/json"
	"net/http"

	"github.com/daniil/billing-service/internal/invoice"
)

// HandleStripeWebhook processes incoming Stripe webhook events for invoice
// lifecycle changes. Idempotent by event id -- Stripe retries delivery on
// timeout, so the same event can arrive more than once.
func HandleStripeWebhook(w http.ResponseWriter, r *http.Request) {
	var evt StripeEvent
	if err := json.NewDecoder(r.Body).Decode(&evt); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	seen, err := eventStore.MarkSeen(r.Context(), evt.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if seen {
		w.WriteHeader(http.StatusOK)
		return
	}

	switch evt.Type {
	case "invoice.paid":
		if err := invoice.MarkPaid(evt.Data.InvoiceID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	case "invoice.payment_failed":
		if err := invoice.MarkFailed(evt.Data.InvoiceID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}
`;

const NEW_WEBHOOK_TEST_GO = `package billing

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleStripeWebhook_IdempotentByEventID(t *testing.T) {
	body := \`{"id":"evt_123","type":"invoice.paid","data":{"invoice_id":"in_1"}}\`

	req1 := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", strings.NewReader(body))
	rec1 := httptest.NewRecorder()
	HandleStripeWebhook(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first delivery: got %d, want 200", rec1.Code)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", strings.NewReader(body))
	rec2 := httptest.NewRecorder()
	HandleStripeWebhook(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("retried delivery: got %d, want 200", rec2.Code)
	}
}
`;

export const HERO_WORKTREE_COMMENT_ID = "wtc-hero-1";

export function buildHeroWorktreeReview(repo: string): WorktreeReview {
  return {
    node_id: WORKTREE_REVIEW_NODE_ID,
    repo,
    worktree_path: "~/.grove/worktrees/99aabbcc-stripe-webhooks",
    branch: "grove/stripe-webhooks",
    base_ref: "master",
    has_uncommitted: true,
    files: [
      {
        path: "internal/billing/webhook.go",
        status: "modified",
        additions: 13,
        deletions: 1,
        binary: false,
        original_content: ORIGINAL_WEBHOOK_GO,
        modified_content: MODIFIED_WEBHOOK_GO,
        content_omitted: "",
        hunks: [],
      },
      {
        path: "internal/billing/webhook_test.go",
        status: "added",
        additions: 26,
        deletions: 0,
        binary: false,
        original_content: "",
        modified_content: NEW_WEBHOOK_TEST_GO,
        content_omitted: "",
        hunks: [],
      },
    ],
  };
}

export function buildHeroWorktreeComment(repo: string): WorktreeComment {
  return {
    id: HERO_WORKTREE_COMMENT_ID,
    node_id: WORKTREE_REVIEW_NODE_ID,
    repo,
    path: "internal/billing/webhook.go",
    line: 20,
    side: "RIGHT",
    body: "Should we log when a duplicate event is dropped? Right now a retried delivery is silent, which will make this annoying to debug from Stripe's dashboard alone.",
    created_at: ago(35 * MIN),
  };
}
