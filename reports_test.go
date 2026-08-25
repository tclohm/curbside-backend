package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func TestCreateReport_Success(t *testing.T) {
	db := newTestDB(t)
	mux := newMux(db)

	rec := doRequest(t, mux, "POST", "/reports", `{
		"plate": "8XYZ123", "color": "Silver",
		"address": "123 Oak St", "issue_type": "Appears abandoned",
		"photo_path": "testdata/fake.jpg"
	}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body)
	}

	var got Report
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if got.ID == "" {
		t.Error("id was empty — expected a generated id")
	}
	if got.Status != "submitted" {
		t.Errorf("status = %q, want %q", got.Status, "submitted")
	}
	if got.CorroborationCount != 1 {
		t.Errorf("corroboration_count = %d, want 1", got.CorroborationCount)
	}
	if got.CreatedAt == "" {
		t.Error("created_at was empty — expected a generated timestamp")
	}
}

// each case is just data (a name, a request body, and the
// status we expect), and one loop runs all of them. t.Run gives each case
// its own named sub-test, so `go test -run TestCreateReport_BadRequest/missing_plate`
// can target just one, and a failure output tells you exactly which case
// failed instead of just "the test failed."
func TestCreateReport_BadRequest(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"malformed JSON", `{not valid json`},
		{"missing plate", `{"color":"Silver","address":"123 Oak St","issue_type":"Appears abandoned"}`},
		{"invalid issue_type", `{"plate":"ZZZ999","color":"Blue","address":"1 Main St","issue_type":"Not A Real Reason"}`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db := newTestDB(t)
			mux := newMux(db)

			rec := doRequest(t, mux, "POST", "/reports", c.body)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body)
			}
		})
	}
}

func TestListReports_EmptyIsArrayNotNull(t *testing.T) {
	db := newTestDB(t)
	mux := newMux(db)

	rec := doRequest(t, mux, "GET", "/reports", "")

	got := rec.Body.String()
	if got != "[]\n" {
		t.Errorf("body = %q, want %q", got, "[]\n")
	}
}

func TestListReports_ReturnsCreated(t *testing.T) {
	db := newTestDB(t)
	mux := newMux(db)

	pendingID1 := insertTestPendingUpload(t, db)
	pendingID2 := insertTestPendingUpload(t, db)
	doRequest(t, mux, "POST", "/reports", fmt.Sprintf(`{"plate":"AAA111","color":"Red","address":"1 First St","issue_type":"Other","pending_upload_id":%q}`, pendingID1))
	doRequest(t, mux, "POST", "/reports", fmt.Sprintf(`{"plate":"BBB222","color":"Blue","address":"2 Second St","issue_type":"Other","pending_upload_id":%q}`, pendingID2))
 
	rec := doRequest(t, mux, "GET", "/reports", "")
 
	var got []Report
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d reports, want 2", len(got))
	}
}

func TestGetReport(t *testing.T) {
	db := newTestDB(t)
	mux := newMux(db)
	pendingID := insertTestPendingUpload(t, db)

	created := doRequest(t, mux, "POST", "/reports", fmt.Sprintf(`{"plate":"8XYZ123","color":"Silver","address":"123 Oak St","issue_type":"Appears abandoned","pending_upload_id":%q}`, pendingID))
	var report Report
	json.Unmarshal(created.Body.Bytes(), &report)

	t.Run("found", func(t *testing.T) {
		rec := doRequest(t, mux, "GET", "/reports/"+report.ID, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
	})

	t.Run("not found", func(t *testing.T) {
		rec := doRequest(t, mux, "GET", "/reports/not-a-real-id", "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})
}
