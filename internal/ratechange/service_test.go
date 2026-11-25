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
		all:     map[int]string{1001: "VEHICULO LIVIANO", 1002: "MOTOCICLETA", 900: "TARIFA ANTIGUA"},
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

	if err := svc.ChangeRate(context.Background(), request(1002)); err != nil {
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

	if err := svc.ChangeRate(context.Background(), request(1002)); err != nil {
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

	if err := svc.ChangeRate(context.Background(), request(1002)); err != nil {
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

			err := svc.ChangeRate(context.Background(), tc.req)

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
	if err := svc.ChangeRate(context.Background(), request(1002)); !errors.Is(err, ErrUnavailable) {
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
