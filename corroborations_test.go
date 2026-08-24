package main

import (
	"encoding/json"
	"net/http"
	"testing"
)

// corroboration test needs a report to corroborate against first.
func createTestReport(t *testing.T, mux *http.ServeMux) Report {
	t.Helper()
	rec := doRequest(t, mux, "POST", "/reports", `{
		"plate": "8XYZ123", "color": "Silver",
		"address": "123 Oak St", "issue_type": "Appears abandoned",
		"photo_path": "testdata/fake.jpg"
	}`)
	var report Report
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatalf("decoding created report: %v", err)
	}
	return report
}

func TestCorroboration_StillThereFlipsStatusAtTwo(t *testing.T) {
	db := newTestDB(t)
	mux := newMux(db)
	report := createTestReport(t, mux)

	rec := doRequest(t, mux, "POST", "/reports/"+report.ID+"/corroborations",
		`{"nonce":"nonce-A","answer":"still-there"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusCreated, rec.Body)
	}

	got := getReportViaMux(t, mux, report.ID)
	if got.CorroborationCount != 2 {
		t.Errorf("corroboration_count = %d, want 2", got.CorroborationCount)
	}
	if got.Status != "under_review" {
		t.Errorf("status = %q, want %q", got.Status, "under_review")
	}
}

func TestCorroboration_SameNonceIsIdempotent(t *testing.T) {
	db := newTestDB(t)
	mux := newMux(db)
	report := createTestReport(t, mux)

	first := doRequest(t, mux, "POST", "/reports/"+report.ID+"/corroborations",
		`{"nonce":"nonce-A","answer":"still-there"}`)
	var firstResp CorroborationResponse
	json.Unmarshal(first.Body.Bytes(), &firstResp)

	second := doRequest(t, mux, "POST", "/reports/"+report.ID+"/corroborations",
		`{"nonce":"nonce-A","answer":"still-there"}`)
	if second.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (idempotent replay, nothing new created)", second.Code, http.StatusOK)
	}
	var secondResp CorroborationResponse
	json.Unmarshal(second.Body.Bytes(), &secondResp)

	if secondResp.ID != firstResp.ID {
		t.Errorf("got a different response id on replay: %q vs %q", secondResp.ID, firstResp.ID)
	}

	got := getReportViaMux(t, mux, report.ID)
	if got.CorroborationCount != 2 {
		t.Errorf("corroboration_count = %d, want 2 (replay must not double-count)", got.CorroborationCount)
	}
}

func TestCorroboration_GoneDoesNotAffectCount(t *testing.T) {
	db := newTestDB(t)
	mux := newMux(db)
	report := createTestReport(t, mux)

	doRequest(t, mux, "POST", "/reports/"+report.ID+"/corroborations",
		`{"nonce":"nonce-B","answer":"gone"}`)

	got := getReportViaMux(t, mux, report.ID)
	if got.CorroborationCount != 1 {
		t.Errorf("corroboration_count = %d, want 1 (unchanged by a 'gone' answer)", got.CorroborationCount)
	}
	if got.Status != "submitted" {
		t.Errorf("status = %q, want %q", got.Status, "submitted")
	}
}

func TestCorroboration_ReportNotFound(t *testing.T) {
	db := newTestDB(t)
	mux := newMux(db)

	rec := doRequest(t, mux, "POST", "/reports/not-a-real-id/corroborations",
		`{"nonce":"x","answer":"still-there"}`)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestCorroboration_BadRequest(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"invalid answer", `{"nonce":"n1","answer":"maybe"}`},
		{"missing nonce", `{"answer":"still-there"}`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db := newTestDB(t)
			mux := newMux(db)
			report := createTestReport(t, mux)

			rec := doRequest(t, mux, "POST", "/reports/"+report.ID+"/corroborations", c.body)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body)
			}
		})
	}
}

// fetches and decodes a report through the real router —
// used by tests above to check a report's state after some action, the
// same way a real client would: by asking the API, not by reaching into
// the database directly.
func getReportViaMux(t *testing.T, mux *http.ServeMux, id string) Report {
	t.Helper()
	rec := doRequest(t, mux, "GET", "/reports/"+id, "")
	var report Report
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatalf("decoding report: %v", err)
	}
	return report
}
