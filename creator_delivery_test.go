package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDeliveryReadiness(t *testing.T) {
	for _, tc := range []struct {
		name, job, verification, want string
		calls                         int
	}{
		{"completed and verified", "completed", "verified", "ready_for_creator_delivery", 4},
		{"completed awaiting DNS", "completed", "pending", "awaiting_domain_verification", 4},
		{"unfinished processing", "running", "verified", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Header.Get("Authorization") != "Bearer test-key" {
					t.Error("missing shared credential")
				}
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/v1/dns/domain/get":
					if r.Method != http.MethodGet || r.URL.Query().Get("domain") != "studio.example" {
						t.Error("domain lookup")
					}
					w.Write([]byte(`{"ok":true,"data":{"zone_id":"zone-1"}}`))
				case "/v1/dns/record/upsert":
					if r.Method != http.MethodPut || r.Header.Get("Idempotency-Key") == "" {
						t.Error("record write")
					}
					w.Write([]byte(`{"ok":true,"data":{}}`))
				case "/v1/email/domain/verify":
					if r.Method != http.MethodPost || r.Header.Get("Idempotency-Key") == "" {
						t.Error("mail verification")
					}
					w.Write([]byte(`{"ok":true,"data":{"verification":{"status":"` + tc.verification + `"}}}`))
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
				}
			}))
			defer server.Close()
			api := domainClient{key: "test-key", base: server.URL, http: server.Client()}
			result, err := prepareDelivery(context.Background(), api, deliveryInput{
				AssetID: "asset-9", JobAssetID: "asset-9", JobID: "transcode-9", JobStatus: tc.job,
				CreatorEmail: "editor@studio.example", Domain: "studio.example", SPF: "v=spf1 include:example.net ~all",
				DKIMName: "mail._domainkey.studio.example", DKIMContent: "mail.example.net",
			})
			if tc.want == "" && err == nil {
				t.Fatal("expected rejection")
			}
			if tc.want != "" && (err != nil || result.State != tc.want || result.ZoneID != "zone-1") {
				t.Fatalf("result=%+v error=%v", result, err)
			}
			if calls != tc.calls {
				t.Fatalf("calls=%d, want %d", calls, tc.calls)
			}
		})
	}
}

func TestBusinessRejectionFromEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"ok":false,"error":{"code":"INVALID_ARGUMENT","message":"check domain"}}`))
	}))
	defer server.Close()
	api := domainClient{key: "test-key", base: server.URL, http: server.Client()}
	_, err := api.zone(context.Background(), "studio.example")
	if err == nil || !strings.Contains(err.Error(), "check domain") {
		t.Fatalf("error=%v", err)
	}
}
