package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andreistefanciprian/yauli/frontend/internal/backendclient"
)

func TestUnsubscribeRejectsMissingParams(t *testing.T) {
	fake := &unsubscribeFakeBackend{}
	h := &Handlers{Backend: fake}

	req := httptest.NewRequest(http.MethodPost, "/unsubscribe?family=f&user=u", nil)
	rec := httptest.NewRecorder()

	h.Unsubscribe(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if fake.called {
		t.Fatal("Backend.Unsubscribe was called despite a missing sig param")
	}
}

func TestUnsubscribePOSTReturnsBareOKOnSuccess(t *testing.T) {
	fake := &unsubscribeFakeBackend{}
	h := &Handlers{Backend: fake}

	req := httptest.NewRequest(http.MethodPost, "/unsubscribe?family=f&user=u&sig=s", nil)
	rec := httptest.NewRecorder()

	h.Unsubscribe(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("POST body = %q, want empty", rec.Body.String())
	}
	if !fake.called || fake.gotFamily != "f" || fake.gotUser != "u" || fake.gotSig != "s" {
		t.Fatalf("Backend.Unsubscribe called with (%q,%q,%q), want (f,u,s)", fake.gotFamily, fake.gotUser, fake.gotSig)
	}
}

func TestUnsubscribeGETReturnsConfirmationPageOnSuccess(t *testing.T) {
	fake := &unsubscribeFakeBackend{}
	h := &Handlers{Backend: fake}

	req := httptest.NewRequest(http.MethodGet, "/unsubscribe?family=f&user=u&sig=s", nil)
	rec := httptest.NewRecorder()

	h.Unsubscribe(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "unsubscribed") {
		t.Fatalf("GET body = %q, want an unsubscribed confirmation", rec.Body.String())
	}
}

func TestUnsubscribeReturnsBadRequestOnBackendError(t *testing.T) {
	fake := &unsubscribeFakeBackend{err: errors.New("invalid signature")}
	h := &Handlers{Backend: fake}

	req := httptest.NewRequest(http.MethodPost, "/unsubscribe?family=f&user=u&sig=s", nil)
	rec := httptest.NewRecorder()

	h.Unsubscribe(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// unsubscribeFakeBackend implements Backend, recording the single call these
// tests care about and erroring on everything else it doesn't stub out.
type unsubscribeFakeBackend struct {
	err error

	called    bool
	gotFamily string
	gotUser   string
	gotSig    string
}

func (f *unsubscribeFakeBackend) Unsubscribe(_ context.Context, family, user, sig string) error {
	f.called = true
	f.gotFamily = family
	f.gotUser = user
	f.gotSig = sig
	return f.err
}

func (f *unsubscribeFakeBackend) GetCurrentUser(context.Context) (backendclient.User, error) {
	return backendclient.User{}, errors.New("not implemented")
}

func (f *unsubscribeFakeBackend) UpdateCurrentUser(context.Context, string) (backendclient.User, error) {
	return backendclient.User{}, errors.New("not implemented")
}

func (f *unsubscribeFakeBackend) GetCurrentBaby(context.Context) (backendclient.Baby, error) {
	return backendclient.Baby{}, errors.New("not implemented")
}

func (f *unsubscribeFakeBackend) CreateBaby(context.Context, string) (backendclient.Baby, error) {
	return backendclient.Baby{}, errors.New("not implemented")
}

func (f *unsubscribeFakeBackend) UpdateCurrentBaby(context.Context, backendclient.Baby) (backendclient.Baby, error) {
	return backendclient.Baby{}, errors.New("not implemented")
}

func (f *unsubscribeFakeBackend) ArchiveCurrentBaby(context.Context, string) error {
	return errors.New("not implemented")
}

func (f *unsubscribeFakeBackend) ListEvents(context.Context, string, string, any) error {
	return errors.New("not implemented")
}

func (f *unsubscribeFakeBackend) GetDailyReport(context.Context, string) (backendclient.DailyReport, error) {
	return backendclient.DailyReport{}, errors.New("not implemented")
}

func (f *unsubscribeFakeBackend) CreateEvent(context.Context, string, map[string]any) error {
	return errors.New("not implemented")
}

func (f *unsubscribeFakeBackend) UpdateEvent(context.Context, string, map[string]any) error {
	return errors.New("not implemented")
}

func (f *unsubscribeFakeBackend) DeleteEvent(context.Context, string) error {
	return errors.New("not implemented")
}

func (f *unsubscribeFakeBackend) InviteHelper(context.Context, string, string) error {
	return errors.New("not implemented")
}

func (f *unsubscribeFakeBackend) ListTimelineMembers(context.Context) (backendclient.TimelineMembersResult, error) {
	return backendclient.TimelineMembersResult{}, errors.New("not implemented")
}

func (f *unsubscribeFakeBackend) UpdateTimelineMemberRelationship(context.Context, string, string) error {
	return errors.New("not implemented")
}

func (f *unsubscribeFakeBackend) UpdateTimelineMemberReportPreferences(context.Context, string, bool) error {
	return errors.New("not implemented")
}

func (f *unsubscribeFakeBackend) RemoveTimelineMember(context.Context, string) error {
	return errors.New("not implemented")
}
