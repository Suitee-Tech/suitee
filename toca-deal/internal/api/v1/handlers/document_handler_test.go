package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/toca-deal/internal/domain/document"
)

func TestRespondDocumentDownloadError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{name: "cross tenant", err: errors.New("access denied"), wantStatus: http.StatusForbidden},
		{name: "missing document", err: errors.New("document not found"), wantStatus: http.StatusNotFound},
		{name: "missing file", err: errors.New("document has no file uploaded"), wantStatus: http.StatusNotFound},
		{name: "storage failure", err: errors.New("supabase sign error: status 503"), wantStatus: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			respondDocumentDownloadError(ctx, tt.err)

			if recorder.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", recorder.Code, tt.wantStatus)
			}
		})
	}
}

func TestApplyAnalysisReference(t *testing.T) {
	t.Parallel()

	t.Run("keeps a general analysis document unscoped", func(t *testing.T) {
		d := &document.Document{EntityID: "deal-1", EntityType: document.EntityAnalysis}
		if err := applyAnalysisReference(d, "", ""); err != nil {
			t.Fatalf("applyAnalysisReference() error = %v", err)
		}
		if d.AnalysisPartyID != "" || d.AnalysisCriterion != "" {
			t.Fatalf("general document was unexpectedly scoped: %+v", d)
		}
	})

	t.Run("links evidence to one party and criterion", func(t *testing.T) {
		d := &document.Document{EntityID: "deal-1", EntityType: document.EntityAnalysis}
		if err := applyAnalysisReference(d, " party-1 ", document.AnalysisCriterionCND); err != nil {
			t.Fatalf("applyAnalysisReference() error = %v", err)
		}
		if d.AnalysisPartyID != "party-1" || d.AnalysisCriterion != document.AnalysisCriterionCND {
			t.Fatalf("scope = (%q, %q), want (party-1, cnd)", d.AnalysisPartyID, d.AnalysisCriterion)
		}
	})

	t.Run("rejects an incomplete or invalid scope", func(t *testing.T) {
		tests := []struct {
			name      string
			doc       *document.Document
			partyID   string
			criterion document.AnalysisCriterion
		}{
			{
				name:      "missing party",
				doc:       &document.Document{EntityID: "deal-1", EntityType: document.EntityAnalysis},
				criterion: document.AnalysisCriterionScore,
			},
			{
				name:    "missing criterion",
				doc:     &document.Document{EntityID: "deal-1", EntityType: document.EntityAnalysis},
				partyID: "party-1",
			},
			{
				name:      "unknown criterion",
				doc:       &document.Document{EntityID: "deal-1", EntityType: document.EntityAnalysis},
				partyID:   "party-1",
				criterion: "unknown",
			},
			{
				name:      "wrong entity type",
				doc:       &document.Document{EntityID: "deal-1", EntityType: document.EntityDeal},
				partyID:   "party-1",
				criterion: document.AnalysisCriterionLegalOpinion,
			},
		}

		for _, tt := range tests {
			tt := tt
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				if err := applyAnalysisReference(tt.doc, tt.partyID, tt.criterion); err == nil {
					t.Fatal("applyAnalysisReference() error = nil, want validation error")
				}
			})
		}
	})
}
