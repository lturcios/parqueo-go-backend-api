package ratechange

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeStore struct {
	docs    map[uint]string
	tickets map[string]Ticket
	writes  map[string]Change
}

func (f *fakeStore) MovementsDoc(ctx context.Context, locationID uint) (string, bool, error) {
	doc, ok := f.docs[locationID]
	return doc, ok, nil
}

func (f *fakeStore) UpdateTicket(ctx context.Context, docID, pagoID string, apply func(Ticket) (Change, error)) error {
	ticket, ok := f.tickets[docID+"/"+pagoID]
	if !ok {
		return ErrNotFound
	}
	change, err := apply(ticket)
	if err != nil {
		return err
	}
	if f.writes == nil {
		f.writes = map[string]Change{}
	}
	f.writes[docID+"/"+pagoID] = change
	return nil
}

type fakeRates struct {
	vehicle map[int]Rate   // current vehicle rates of location 8
	all     map[int]string // every rate code of location 8
}

func (f fakeRates) VehicleRate(code int, locationID uint) (Rate, bool, error) {
	rate, ok := f.vehicle[code]
	return rate, ok && locationID == 8, nil
}

func (f fakeRates) Description(code int, locationID uint) (string, error) {
	return f.all[code], nil
}

func newService(ticket Ticket) (*Service, *fakeStore) {
	store := &fakeStore{
		docs:    map[uint]string{8: "LT-8"},
		tickets: map[string]Ticket{"LT-8/G5V._iBQMhib04_P": ticket},
	}
	rates := fakeRates{
		vehicle: map[int]Rate{
			1001: {Description: "VEHICULO LIVIANO", UnitPrice: 1.50},
			1002: {Description: "MOTOCICLETA", UnitPrice: 1.00},
		},
		all: map[int]string{1001: "VEHICULO LIVIANO", 1002: "MOTOCICLETA", 900: "TARIFA ANTIGUA"},
	}
	now := func() time.Time { return time.Date(2026, 11, 24, 23, 20, 0, 0, time.Local) }
	return &Service{Store: store, Rates: rates, Now: now}, store
}

func activeTicket() Ticket {
	return Ticket{RateCode: 1001, Active: true}
}

func request(newCode int) Request {
	return Request{LocationID: 8, PagoID: "G5V._iBQMhib04_P", NewRateCode: newCode, UserEmail: "admin@example.com"}
}

func TestChangeRateUpdatesOnlyRateAndAppendsNote(t *testing.T) {
	ticket := activeTicket()
	ticket.Observations = "CLIENTE FRECUENTE"
	svc, store := newService(ticket)

	if _, err := svc.ChangeRate(context.Background(), request(1002)); err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	got := store.writes["LT-8/G5V._iBQMhib04_P"]
	want := Change{
		RateCode:     1002,
		UnitPrice:    1.00,
		Observations: "CLIENTE FRECUENTE | Tarifa cambiada de VEHICULO LIVIANO a MOTOCICLETA por admin@example.com el 2026-11-24 23:20",
	}
	if got != want {
		t.Fatalf("unexpected write:\n got %+v\nwant %+v", got, want)
	}
}

func TestChangeRateNoteWithoutPreviousObservations(t *testing.T) {
	svc, store := newService(activeTicket())

	if _, err := svc.ChangeRate(context.Background(), request(1002)); err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	want := "Tarifa cambiada de VEHICULO LIVIANO a MOTOCICLETA por admin@example.com el 2026-11-24 23:20"
	if got := store.writes["LT-8/G5V._iBQMhib04_P"].Observations; got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestChangeRateDescribesRetiredCurrentRate(t *testing.T) {
	ticket := activeTicket()
	ticket.RateCode = 900
	svc, store := newService(ticket)

	if _, err := svc.ChangeRate(context.Background(), request(1002)); err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	if got := store.writes["LT-8/G5V._iBQMhib04_P"].Observations; !strings.HasPrefix(got, "Tarifa cambiada de TARIFA ANTIGUA a MOTOCICLETA") {
		t.Fatalf("expected the retired rate description, got %q", got)
	}
}

func TestChangeRateRejections(t *testing.T) {
	closed := activeTicket()
	closed.Active = false
	otros := activeTicket()
	otros.OtherIncome = true

	cases := []struct {
		name   string
		ticket Ticket
		req    Request
		want   error
	}{
		{"closed vehicle", closed, request(1002), ErrNotActive},
		{"other income ticket", otros, request(1002), ErrNotVehicle},
		{"same rate", activeTicket(), request(1001), ErrSameRate},
		{"unknown rate", activeTicket(), request(7777), ErrInvalidRate},
		{"location without Firestore", activeTicket(), Request{LocationID: 5, PagoID: "G5V._iBQMhib04_P", NewRateCode: 1002}, ErrUnavailable},
		{"missing movement", activeTicket(), Request{LocationID: 8, PagoID: "doesNotExist", NewRateCode: 1002}, ErrNotFound},
		{"path in pago id", activeTicket(), Request{LocationID: 8, PagoID: "../LT-1/x", NewRateCode: 1002}, ErrNotFound},
		{"reserved pago id", activeTicket(), Request{LocationID: 8, PagoID: "__name__", NewRateCode: 1002}, ErrNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, store := newService(tc.ticket)

			_, err := svc.ChangeRate(context.Background(), tc.req)

			if !errors.Is(err, tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, err)
			}
			if len(store.writes) != 0 {
				t.Fatalf("expected no write, got %+v", store.writes)
			}
		})
	}
}

func TestServiceWithoutStoreIsUnavailable(t *testing.T) {
	svc := &Service{Rates: fakeRates{}, Now: time.Now}

	available, err := svc.Available(context.Background(), 8)
	if err != nil || available {
		t.Fatalf("expected unavailable without error, got %v, %v", available, err)
	}
	if _, err := svc.ChangeRate(context.Background(), request(1002)); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
}

func TestAvailableFollowsFirestoreConfig(t *testing.T) {
	svc, _ := newService(activeTicket())

	if ok, _ := svc.Available(context.Background(), 8); !ok {
		t.Fatal("expected location 8 to be available")
	}
	if ok, _ := svc.Available(context.Background(), 5); ok {
		t.Fatal("expected location 5 to be unavailable")
	}
}

func TestAppendNoteKeepsWithinColumnLimit(t *testing.T) {
	note := "Tarifa cambiada de VEHICULO LIVIANO a MOTOCICLETA por admin@example.com el 2026-11-24 23:20"
	existing := strings.Repeat("A", 180)

	got := appendNote(existing, note)

	if n := len([]rune(got)); n != maxObservations {
		t.Fatalf("expected %d characters, got %d", maxObservations, n)
	}
	if !strings.HasSuffix(got, " | "+note) {
		t.Fatalf("expected the new note intact at the end, got %q", got)
	}
	if !strings.HasPrefix(got, "...") {
		t.Fatalf("expected trimmed older text to start with ..., got %q", got)
	}
}

func TestAppendNoteTruncatesOversizedNote(t *testing.T) {
	note := strings.Repeat("N", 250)

	got := appendNote("previous", note)

	if n := len([]rune(got)); n != maxObservations {
		t.Fatalf("expected %d characters, got %d", maxObservations, n)
	}
}

type fakeMirror struct {
	updated bool
	err     error
	calls   []Change
}

func (f *fakeMirror) ApplyRateChange(locationID uint, pagoID string, change Change) (bool, error) {
	if locationID != 8 || pagoID != "G5V._iBQMhib04_P" {
		return false, errors.New("unexpected ticket")
	}
	f.calls = append(f.calls, change)
	return f.updated, f.err
}

func TestChangeRateMirrorsTheStoredChangeToMySQL(t *testing.T) {
	svc, store := newService(activeTicket())
	mirror := &fakeMirror{updated: true}
	svc.Mirror = mirror

	result, err := svc.ChangeRate(context.Background(), request(1002))

	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	written := store.writes["LT-8/G5V._iBQMhib04_P"]
	if len(mirror.calls) != 1 || mirror.calls[0] != written {
		t.Fatalf("expected MySQL to receive the Firestore change %+v, got %+v", written, mirror.calls)
	}
	if !result.MySQLUpdated || result.MySQLErr != nil {
		t.Fatalf("expected MySQL updated without error, got %+v", result)
	}
}

func TestChangeRateWithoutMySQLRowStillSucceeds(t *testing.T) {
	svc, _ := newService(activeTicket())
	svc.Mirror = &fakeMirror{updated: false}

	result, err := svc.ChangeRate(context.Background(), request(1002))

	if err != nil || result.MySQLUpdated || result.MySQLErr != nil {
		t.Fatalf("expected success with no MySQL row updated, got %+v, %v", result, err)
	}
}

func TestChangeRateReportsMySQLFailureAfterFirestoreWrite(t *testing.T) {
	svc, store := newService(activeTicket())
	svc.Mirror = &fakeMirror{err: errors.New("mysql down")}

	result, err := svc.ChangeRate(context.Background(), request(1002))

	if err != nil {
		t.Fatalf("expected the Firestore change to count as success, got %v", err)
	}
	if len(store.writes) != 1 {
		t.Fatalf("expected the Firestore write to stay, got %+v", store.writes)
	}
	if result.MySQLUpdated || result.MySQLErr == nil {
		t.Fatalf("expected the MySQL error to be reported, got %+v", result)
	}
}

func TestChangeRateDoesNotTouchMySQLWhenFirestoreRejects(t *testing.T) {
	closed := activeTicket()
	closed.Active = false
	svc, _ := newService(closed)
	mirror := &fakeMirror{updated: true}
	svc.Mirror = mirror

	if _, err := svc.ChangeRate(context.Background(), request(1002)); !errors.Is(err, ErrNotActive) {
		t.Fatalf("expected ErrNotActive, got %v", err)
	}
	if len(mirror.calls) != 0 {
		t.Fatalf("expected no MySQL update, got %+v", mirror.calls)
	}
}

// retryingStore runs apply twice, like a Firestore transaction retried after contention.
type retryingStore struct {
	fakeStore
}

func (r *retryingStore) UpdateTicket(ctx context.Context, docID, pagoID string, apply func(Ticket) (Change, error)) error {
	first := r.tickets[docID+"/"+pagoID]
	if _, err := apply(first); err != nil {
		return err
	}
	second := first
	second.Observations = "NOTA DEL DISPOSITIVO"
	change, err := apply(second)
	if err != nil {
		return err
	}
	r.writes = map[string]Change{docID + "/" + pagoID: change}
	return nil
}

func TestChangeRateMirrorsTheCommittedAttemptAfterRetry(t *testing.T) {
	svc, base := newService(activeTicket())
	store := &retryingStore{fakeStore: *base}
	svc.Store = store
	mirror := &fakeMirror{updated: true}
	svc.Mirror = mirror

	if _, err := svc.ChangeRate(context.Background(), request(1002)); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	committed := store.writes["LT-8/G5V._iBQMhib04_P"]
	if len(mirror.calls) != 1 || mirror.calls[0] != committed {
		t.Fatalf("expected MySQL to get the committed change %+v, got %+v", committed, mirror.calls)
	}
}
