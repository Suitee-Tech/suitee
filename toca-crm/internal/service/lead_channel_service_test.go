package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/oginoino/toca/pkg/clients"
	pkgcontext "github.com/oginoino/toca/pkg/context"
)

func TestLeadChannelChecksOwnershipBeforeCallingComm(t *testing.T) {
	for _, tc := range []struct {
		name, accountID, email, leadID string
		wantErr                        error
		participants                   []string
	}{
		{name: "owner", accountID: "client", leadID: "lead", participants: []string{"broker", "client"}},
		{name: "alternative authenticated identity", accountID: "otp-client", email: " CLIENT@example.test ", leadID: "lead", participants: []string{"broker", "otp-client", "client"}},
		{name: "unrelated client", accountID: "stranger", email: "other@example.test", leadID: "lead", wantErr: ErrVisitForbidden},
		{name: "missing account even with matching email", email: "client@example.test", leadID: "lead", wantErr: ErrVisitForbidden},
		{name: "missing email fallback", accountID: "stranger", leadID: "lead", wantErr: ErrVisitForbidden},
		{name: "unknown lead", accountID: "client", leadID: "unknown", wantErr: ErrVisitNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, _, _ := visitFixture()
			calls := 0
			var request clients.EnsureChannelRequest
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"data":{"id":"channel"}}`))
			}))
			defer server.Close()
			base := clients.NewBaseClient(server.URL)
			base.InternalAPIKey = "test-key"
			svc.WithCommClient(&clients.CommClient{BaseClient: base})
			ctx := pkgcontext.WithAccountID(context.Background(), tc.accountID)
			ctx = pkgcontext.WithUserEmail(ctx, tc.email)
			channelID, err := svc.EnsureDirectChannel(ctx, tc.leadID)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("got %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil {
				if calls != 0 || channelID != "" {
					t.Fatalf("unauthorized/missing lead contacted comm: calls=%d channel=%q", calls, channelID)
				}
				return
			}
			if calls != 1 || channelID != "channel" || !reflect.DeepEqual(request.Participants, tc.participants) {
				t.Fatalf("invalid authorized conversation: calls=%d channel=%q participants=%v", calls, channelID, request.Participants)
			}
		})
	}
}
