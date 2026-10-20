package proof_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"

	"github.com/devanshbaghel18/ONUSLY/internal/goals"
	"github.com/devanshbaghel18/ONUSLY/internal/middleware"
	"github.com/devanshbaghel18/ONUSLY/internal/proof"
)

func cleanupProofAndGoal(t *testing.T, db *dynamodb.Client, tableName, ownerID, goalID string) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, _ = db.DeleteItem(ctx, &dynamodb.DeleteItemInput{
			TableName: &tableName,
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: "USER#" + ownerID},
				"SK": &types.AttributeValueMemberS{Value: "GOAL#" + goalID},
			},
		})
	})
}

// TestProofSecurity_UnrelatedUserBlockedFromViewingProofs verifies that:
// An authenticated caller who is neither the owner nor the assigned approver cannot list or read proofs.
func TestProofSecurity_UnrelatedUserBlockedFromViewingProofs(t *testing.T) {
	db, tableName := getTestDynamoClient(t)

	ownerID, approverID, goalID := setupTestGoal(t, db, tableName, "active")
	cleanupProofAndGoal(t, db, tableName, ownerID, goalID)

	goalRepo := goals.NewRepository(db, tableName)
	goalService := goals.NewService(goalRepo, nil)

	proofRepo := proof.NewRepository(db, tableName)
	proofService := proof.NewService(proofRepo, goalService)
	handler := proof.NewHandler(proofService)

	// Owner submits proof
	p, err := proofService.Submit(
		context.Background(),
		ownerID,
		goalID,
		"text",
		"Completed my homework",
		"",
		"",
	)
	if err != nil {
		t.Fatalf("failed to submit proof: %v", err)
	}

	eveID := "test-unrelated-eve-" + uuid.NewString()

	// 1. Eve attempts GET /goals/{id}/proofs without query param
	{
		url := fmt.Sprintf("/goals/%s/proofs", goalID)
		req := httptest.NewRequest("GET", url, nil)
		req.SetPathValue("id", goalID)
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, eveID))
		rec := httptest.NewRecorder()

		handler.ListByGoal(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden for unrelated user listing proofs, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	}

	// 2. Eve attempts GET /goals/{id}/proofs?ownerId=<alice> (attempted IDOR bypass)
	{
		url := fmt.Sprintf("/goals/%s/proofs?ownerId=%s", goalID, ownerID)
		req := httptest.NewRequest("GET", url, nil)
		req.SetPathValue("id", goalID)
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, eveID))
		rec := httptest.NewRecorder()

		handler.ListByGoal(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden for forged ownerId query param, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	}

	// 3. Eve attempts GET /goals/{id}/proofs/{proofId}
	{
		url := fmt.Sprintf("/goals/%s/proofs/%s", goalID, p.ID)
		req := httptest.NewRequest("GET", url, nil)
		req.SetPathValue("id", goalID)
		req.SetPathValue("proofId", p.ID)
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, eveID))
		rec := httptest.NewRecorder()

		handler.GetByID(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden for unrelated user getting proof by ID, got %d (body: %s)", rec.Code, rec.Body.String())
		}
	}

	// 4. Assigned approver successfully lists and reads proof
	{
		url := fmt.Sprintf("/goals/%s/proofs", goalID)
		req := httptest.NewRequest("GET", url, nil)
		req.SetPathValue("id", goalID)
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, approverID))
		rec := httptest.NewRecorder()

		handler.ListByGoal(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for assigned approver listing proofs, got %d (body: %s)", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), p.ID) {
			t.Fatalf("expected proof ID in approver's response, got: %s", rec.Body.String())
		}
	}

	// 5. Assigned approver gets specific proof by ID
	{
		url := fmt.Sprintf("/goals/%s/proofs/%s", goalID, p.ID)
		req := httptest.NewRequest("GET", url, nil)
		req.SetPathValue("id", goalID)
		req.SetPathValue("proofId", p.ID)
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, approverID))
		rec := httptest.NewRecorder()

		handler.GetByID(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for assigned approver fetching proof by ID, got %d (body: %s)", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "Completed my homework") {
			t.Fatalf("expected proof content in approver's response, got: %s", rec.Body.String())
		}
	}
}

// TestProofSecurity_UnrelatedUserCannotSubmitProofForAnotherUsersGoal verifies that:
// Eve cannot submit proof for a goal owned by Alice.
func TestProofSecurity_UnrelatedUserCannotSubmitProofForAnotherUsersGoal(t *testing.T) {
	db, tableName := getTestDynamoClient(t)

	ownerID, _, goalID := setupTestGoal(t, db, tableName, "active")
	cleanupProofAndGoal(t, db, tableName, ownerID, goalID)

	goalRepo := goals.NewRepository(db, tableName)
	goalService := goals.NewService(goalRepo, nil)

	proofRepo := proof.NewRepository(db, tableName)
	proofService := proof.NewService(proofRepo, goalService)
	handler := proof.NewHandler(proofService)

	eveID := "test-unrelated-eve-" + uuid.NewString()

	body := strings.NewReader(`{"proofType":"text","textExplanation":"Faked proof"}`)
	url := fmt.Sprintf("/goals/%s/proofs", goalID)
	req := httptest.NewRequest("POST", url, body)
	req.SetPathValue("id", goalID)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserIDKey, eveID))
	rec := httptest.NewRecorder()

	handler.Submit(rec, req)

	// Goal not found in Eve's partition
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusForbidden {
		t.Fatalf("expected 404 Not Found or 403 Forbidden for Eve submitting proof on Alice's goal, got %d: %s", rec.Code, rec.Body.String())
	}
}
