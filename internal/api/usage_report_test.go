package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestUsageReportHTTPContract(testingHandle *testing.T) {
	database := memory.New()
	server := New(Config{Store: database, Token: "usage-test"})
	testutil.RequireNoError(testingHandle, database.Usage().Save(context.Background(), atom.UsageRecord{InstanceID: "workspace", ModelID: "synthetic/model", CreatedAt: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC), Usage: atom.Usage{Input: 13, CacheRead: 20, Output: 7}}))
	for _, test := range []struct {
		query  string
		status int
	}{
		{"from=2026-03-01&to=2026-03-01&instance_id=workspace", http.StatusOK},
		{"from=2026-03-01&to=2026-03-01&instance_id=other", http.StatusOK},
		{"from=invalid", http.StatusBadRequest}, {"from=2026-02-30", http.StatusBadRequest},
		{"from=2026-03-02&to=2026-03-01", http.StatusBadRequest},
		{"from=2025-01-01&to=2026-03-01", http.StatusBadRequest},
		{"instance_id=a&session_id=b", http.StatusBadRequest}, {"agent_prefix=context.", http.StatusBadRequest},
		{"key_id=not-recorded", http.StatusBadRequest}, {"from=2026-03-01&from=2026-03-02", http.StatusBadRequest},
	} {
		request := httptest.NewRequest(http.MethodGet, "/usage?"+test.query, nil)
		request.Header.Set("Authorization", "Bearer usage-test")
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != test.status {
			testingHandle.Fatalf("%s: %d %s", test.query, response.Code, response.Body.String())
		}
		if response.Code == http.StatusOK {
			var report atom.UsageReport
			testutil.RequireNoError(testingHandle, json.Unmarshal(response.Body.Bytes(), &report))
			if report.Buckets == nil {
				testingHandle.Fatal("buckets must be an array")
			}
			if test.query == "from=2026-03-01&to=2026-03-01&instance_id=workspace" && (report.Statistics.Input != 13 || report.Statistics.CacheRead != 20 || report.Statistics.Output != 7 || report.Statistics.Cost != nil) {
				testingHandle.Fatalf("wire usage: %+v", report)
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				testingHandle.Fatal("usage responses must not be retained by the browser cache")
			}
		}
	}
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/usage", nil))
	if response.Code != http.StatusUnauthorized {
		testingHandle.Fatalf("unauthenticated usage: %d", response.Code)
	}
	query, operationError := parseUsageQuery(httptest.NewRequest(http.MethodGet, "/usage?to=2026-03-01", nil), time.Now())
	testutil.RequireNoError(testingHandle, operationError)
	if query.From.Format(time.DateOnly) != "2026-01-31" || query.To.Format(time.DateOnly) != "2026-03-01" {
		testingHandle.Fatalf("default inclusive range: %+v", query)
	}
}
