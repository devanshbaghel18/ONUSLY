package proof_test

import (
	"context"
	"strings"
	"testing"

	"github.com/devanshbaghel18/ONUSLY/internal/goals"
	"github.com/devanshbaghel18/ONUSLY/internal/proof"
)

func TestD13_ProofValidationSecurity(t *testing.T) {
	activeGoal := goals.Goal{
		ID:         "goal-sec-1",
		OwnerID:    "user-sec-1",
		Status:     "active",
		ApproverID: "approver-1",
	}

	goalRepo := newD11MockGoalRepo()
	_ = goalRepo.Create(context.Background(), activeGoal)
	goalService := goals.NewService(goalRepo, nil)

	proofRepo := newD11MockProofRepo(goalRepo)
	svc := proof.NewService(proofRepo, goalService)

	ctx := context.Background()

	// 1. External link must not be javascript: scheme
	_, err := svc.Submit(ctx, "user-sec-1", "goal-sec-1", "link", "", "javascript:alert(document.cookie)", "")
	if err == nil || !strings.Contains(err.Error(), "must be a valid http:// or https:// URL") {
		t.Fatalf("expected rejection of javascript: external link, got: %v", err)
	}

	// 2. External link must not be data: scheme
	_, err = svc.Submit(ctx, "user-sec-1", "goal-sec-1", "link", "", "data:text/html,<script>alert(1)</script>", "")
	if err == nil || !strings.Contains(err.Error(), "must be a valid http:// or https:// URL") {
		t.Fatalf("expected rejection of data: external link, got: %v", err)
	}

	// 3. External link length limit (2048 chars)
	oversizedLink := "https://example.com/" + strings.Repeat("a", 2050)
	_, err = svc.Submit(ctx, "user-sec-1", "goal-sec-1", "link", "", oversizedLink, "")
	if err == nil || !strings.Contains(err.Error(), "exceeds maximum length of 2048 characters") {
		t.Fatalf("expected rejection of oversized link, got: %v", err)
	}

	// 4. Photo URL must reject javascript: scheme
	_, err = svc.Submit(ctx, "user-sec-1", "goal-sec-1", "photo", "", "", "javascript:alert(1)")
	if err == nil || !strings.Contains(err.Error(), "must be an http(s) URL or valid image data URI") {
		t.Fatalf("expected rejection of javascript: photo URL, got: %v", err)
	}

	// 5. Photo URL with invalid MIME data URI (e.g. executable or html)
	_, err = svc.Submit(ctx, "user-sec-1", "goal-sec-1", "photo", "", "", "data:text/html;base64,PHNjcmlwdD4=")
	if err == nil || (!strings.Contains(err.Error(), "valid image data URI") && !strings.Contains(err.Error(), "valid MIME prefix")) {
		t.Fatalf("expected rejection of non-image MIME data URI, got: %v", err)
	}

	// 6. Photo URL data URI exceeding 2MB
	oversizedDataURI := "data:image/png;base64," + strings.Repeat("A", 2*1024*1024+10)
	_, err = svc.Submit(ctx, "user-sec-1", "goal-sec-1", "photo", "", "", oversizedDataURI)
	if err == nil || !strings.Contains(err.Error(), "exceeds maximum size limit of 2MB") {
		t.Fatalf("expected rejection of oversized data URI, got: %v", err)
	}

	// 7. Valid http link succeeds
	p, err := svc.Submit(ctx, "user-sec-1", "goal-sec-1", "link", "verification details", "https://github.com/onusly/proof", "")
	if err != nil || p == nil {
		t.Fatalf("valid link submission failed: %v", err)
	}
}
