package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/andreistefanciprian/yauli/backend-api/internal/store"
)

func TestSignAndVerifyUnsubscribeToken(t *testing.T) {
	familyID := uuid.New()
	userID := uuid.New()
	sig := signUnsubscribeToken("secret", familyID, userID)

	if !verifyUnsubscribeToken("secret", familyID, userID, sig) {
		t.Fatal("verifyUnsubscribeToken rejected a signature it just produced")
	}
	if verifyUnsubscribeToken("other-secret", familyID, userID, sig) {
		t.Fatal("verifyUnsubscribeToken accepted a signature signed with a different secret")
	}
	if verifyUnsubscribeToken("secret", uuid.New(), userID, sig) {
		t.Fatal("verifyUnsubscribeToken accepted a signature for a different family")
	}
	if verifyUnsubscribeToken("secret", familyID, uuid.New(), sig) {
		t.Fatal("verifyUnsubscribeToken accepted a signature for a different user")
	}
}

func TestUnsubscribeURL(t *testing.T) {
	familyID := uuid.New()
	userID := uuid.New()

	if got := unsubscribeURL("", "secret", familyID, userID); got != "" {
		t.Fatalf("unsubscribeURL with empty frontend URL = %q, want empty", got)
	}
	if got := unsubscribeURL("https://getyauli.com", "", familyID, userID); got != "" {
		t.Fatalf("unsubscribeURL with empty secret = %q, want empty", got)
	}

	got := unsubscribeURL("https://getyauli.com/", "secret", familyID, userID)
	want := "https://getyauli.com/unsubscribe?family=" + familyID.String() + "&user=" + userID.String() + "&sig=" + signUnsubscribeToken("secret", familyID, userID)
	if got != want {
		t.Fatalf("unsubscribeURL = %q, want %q", got, want)
	}
}

func TestUnsubscribeReportEmailFlipsPreferenceOnValidSignature(t *testing.T) {
	familyID := uuid.New()
	userID := uuid.New()
	fake := &unsubscribeFakeFamilyStore{}
	h := &Handlers{FamilyStore: fake, UnsubscribeSecret: "secret"}

	sig := signUnsubscribeToken("secret", familyID, userID)
	body := `{"family_id":"` + familyID.String() + `","user_id":"` + userID.String() + `","sig":"` + sig + `"}`
	req := httptest.NewRequest(http.MethodPost, "/email-preferences/unsubscribe", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.UnsubscribeReportEmail(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	if !fake.called {
		t.Fatal("UpdateDailyReportEmailPreference was not called")
	}
	if fake.gotFamilyID != familyID || fake.gotUserID != userID || fake.gotEnabled != false {
		t.Fatalf("UpdateDailyReportEmailPreference called with (%s, %s, %v), want (%s, %s, false)",
			fake.gotFamilyID, fake.gotUserID, fake.gotEnabled, familyID, userID)
	}
}

func TestUnsubscribeReportEmailRejectsTamperedSignature(t *testing.T) {
	familyID := uuid.New()
	userID := uuid.New()
	fake := &unsubscribeFakeFamilyStore{}
	h := &Handlers{FamilyStore: fake, UnsubscribeSecret: "secret"}

	body := `{"family_id":"` + familyID.String() + `","user_id":"` + userID.String() + `","sig":"not-the-right-signature"}`
	req := httptest.NewRequest(http.MethodPost, "/email-preferences/unsubscribe", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.UnsubscribeReportEmail(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if fake.called {
		t.Fatal("UpdateDailyReportEmailPreference was called despite an invalid signature")
	}
}

func TestUnsubscribeReportEmailRejectsWhenSecretUnconfigured(t *testing.T) {
	familyID := uuid.New()
	userID := uuid.New()
	fake := &unsubscribeFakeFamilyStore{}
	h := &Handlers{FamilyStore: fake, UnsubscribeSecret: ""}

	sig := signUnsubscribeToken("secret", familyID, userID)
	body := `{"family_id":"` + familyID.String() + `","user_id":"` + userID.String() + `","sig":"` + sig + `"}`
	req := httptest.NewRequest(http.MethodPost, "/email-preferences/unsubscribe", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.UnsubscribeReportEmail(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if fake.called {
		t.Fatal("UpdateDailyReportEmailPreference was called with no signing secret configured")
	}
}

func TestUnsubscribeReportEmailTreatsNotFoundAsSuccess(t *testing.T) {
	familyID := uuid.New()
	userID := uuid.New()
	fake := &unsubscribeFakeFamilyStore{err: store.ErrNotFound}
	h := &Handlers{FamilyStore: fake, UnsubscribeSecret: "secret"}

	sig := signUnsubscribeToken("secret", familyID, userID)
	body := `{"family_id":"` + familyID.String() + `","user_id":"` + userID.String() + `","sig":"` + sig + `"}`
	req := httptest.NewRequest(http.MethodPost, "/email-preferences/unsubscribe", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.UnsubscribeReportEmail(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (already unsubscribed/removed should read as success)", rec.Code)
	}
}

func TestUnsubscribeReportEmailRejectsInvalidJSON(t *testing.T) {
	h := &Handlers{FamilyStore: &unsubscribeFakeFamilyStore{}, UnsubscribeSecret: "secret"}

	req := httptest.NewRequest(http.MethodPost, "/email-preferences/unsubscribe", strings.NewReader("not json"))
	rec := httptest.NewRecorder()

	h.UnsubscribeReportEmail(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// unsubscribeFakeFamilyStore implements FamilyStore, recording the single
// call these tests care about and erroring on everything else it doesn't
// stub out, matching the fake-store idiom used elsewhere in this package
// (e.g. aiReportFakeStore in report_ai_test.go).
type unsubscribeFakeFamilyStore struct {
	err error

	called      bool
	gotFamilyID uuid.UUID
	gotUserID   uuid.UUID
	gotEnabled  bool
}

func (f *unsubscribeFakeFamilyStore) UpdateDailyReportEmailPreference(_ context.Context, familyID, userID uuid.UUID, enabled bool) (store.FamilyMembership, error) {
	f.called = true
	f.gotFamilyID = familyID
	f.gotUserID = userID
	f.gotEnabled = enabled
	if f.err != nil {
		return store.FamilyMembership{}, f.err
	}
	return store.FamilyMembership{}, nil
}

func (f *unsubscribeFakeFamilyStore) UpsertUserByEmail(context.Context, string) (store.User, error) {
	return store.User{}, errors.New("not implemented")
}

func (f *unsubscribeFakeFamilyStore) GetUser(context.Context, uuid.UUID) (store.User, error) {
	return store.User{}, errors.New("not implemented")
}

func (f *unsubscribeFakeFamilyStore) UpdateUserDisplayName(context.Context, uuid.UUID, string) (store.User, error) {
	return store.User{}, errors.New("not implemented")
}

func (f *unsubscribeFakeFamilyStore) GetFamilyMembership(context.Context, uuid.UUID) (store.FamilyMembership, error) {
	return store.FamilyMembership{}, errors.New("not implemented")
}

func (f *unsubscribeFakeFamilyStore) GetFamilyMembershipForFamily(context.Context, uuid.UUID, uuid.UUID) (store.FamilyMembership, error) {
	return store.FamilyMembership{}, errors.New("not implemented")
}

func (f *unsubscribeFakeFamilyStore) HasPendingInviteOutsideFamily(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return false, errors.New("not implemented")
}

func (f *unsubscribeFakeFamilyStore) CreateFamilyWithOwner(context.Context, uuid.UUID, string) (uuid.UUID, error) {
	return uuid.UUID{}, errors.New("not implemented")
}

func (f *unsubscribeFakeFamilyStore) ActivateInvitedMembership(context.Context, uuid.UUID, uuid.UUID) error {
	return errors.New("not implemented")
}

func (f *unsubscribeFakeFamilyStore) CreateInvite(context.Context, uuid.UUID, string) error {
	return errors.New("not implemented")
}

func (f *unsubscribeFakeFamilyStore) ListTimelineMembers(context.Context, uuid.UUID) ([]store.TimelineMember, error) {
	return nil, errors.New("not implemented")
}

func (f *unsubscribeFakeFamilyStore) UpdateTimelineMemberRelationship(context.Context, uuid.UUID, uuid.UUID, string) error {
	return errors.New("not implemented")
}

func (f *unsubscribeFakeFamilyStore) RemoveTimelineMember(context.Context, uuid.UUID, uuid.UUID) error {
	return errors.New("not implemented")
}
