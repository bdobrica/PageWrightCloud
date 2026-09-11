package handlers

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/auth"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/database"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/middleware"
	"github.com/bdobrica/PageWrightCloud/pagewright/gateway/internal/types"
)

// Unimplemented embedded methods panic if rejection accidentally reaches dispatch.
type pilotBuildStore struct {
	buildStore
	prior *database.BuildSubmission
}

func (*pilotBuildStore) GetSiteByFQDNContext(context.Context, string) (*types.Site, error) {
	return &types.Site{ID: "site", UserID: "owner"}, nil
}
func (s *pilotBuildStore) FindBuildSubmission(context.Context, string, string, string) (*database.BuildSubmission, error) {
	return s.prior, nil
}

type deniedAdmission struct {
	err   error
	calls int
}

func (s *deniedAdmission) AdmitPilot(context.Context, string, string, string, string, database.PilotLimits) error {
	s.calls++
	return s.err
}
func (*deniedAdmission) ReleasePilot(context.Context, string, string, string) error {
	panic("releasing unadmitted attempt")
}

func TestPilotBuildRejectsBeforeProviderOrDispatch(t *testing.T) {
	for _, tc := range []struct {
		name          string
		allowance     int
		err           error
		status, calls int
	}{
		{"disabled", 0, nil, 429, 0},
		{"quota", 1000, database.ErrPilotLimit, 429, 1},
		{"conflict", 1000, database.ErrSubmissionConflict, 409, 1},
		{"database unavailable", 1000, errors.New("private database error"), 503, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			admission := &deniedAdmission{err: tc.err}
			h := NewBuildHandler(&pilotBuildStore{}, nil, nil, nil)
			h.SetPilotLimits(admission, database.PilotLimits{UserDaily: 10, SiteDaily: 5, Active: 2})
			h.SetPilotAIAllowance(tc.allowance)
			r := httptest.NewRequest("POST", "/sites/site/build", strings.NewReader(`{"message":"edit"}`))
			r.Header.Set("Idempotency-Key", "11111111-1111-4111-8111-111111111111")
			r = r.WithContext(context.WithValue(r.Context(), middleware.UserContextKey, &auth.Claims{UserID: "owner"}))
			w := httptest.NewRecorder()
			h.Build(w, r)
			if w.Code != tc.status || admission.calls != tc.calls {
				t.Fatalf("status=%d calls=%d body=%s", w.Code, admission.calls, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "private database") {
				t.Fatal("leaked diagnostic")
			}
			if tc.status == 429 && w.Header().Get("Retry-After") == "" {
				t.Fatal("missing retry guidance")
			}
		})
	}
}

func TestPilotCommittedRetryBypassesAllowanceAndAdmission(t *testing.T) {
	body := `{"message":"edit"}`
	prior := &database.BuildSubmission{RequestHash: fmt.Sprintf("%x", sha256.Sum256([]byte(body))), DispatchState: "accepted", Status: "completed", ResponseStatus: 200}
	admission := &deniedAdmission{err: database.ErrPilotLimit}
	h := NewBuildHandler(&pilotBuildStore{prior: prior}, nil, nil, nil)
	h.SetPilotLimits(admission, database.PilotLimits{})
	h.SetPilotAIAllowance(0)
	r := httptest.NewRequest("POST", "/sites/site/build", strings.NewReader(body))
	r.Header.Set("Idempotency-Key", "11111111-1111-4111-8111-111111111111")
	r = r.WithContext(context.WithValue(r.Context(), middleware.UserContextKey, &auth.Claims{UserID: "owner"}))
	w := httptest.NewRecorder()
	h.Build(w, r)
	if w.Code != 200 || admission.calls != 0 {
		t.Fatalf("status=%d calls=%d body=%s", w.Code, admission.calls, w.Body.String())
	}
}
