// Package ratechange holds the rules for correcting the rate of an active ticket.
// Tickets of Firestore clients live in Firestore: the device sees the change through its
// realtime listener, so the change is written there first. It is then mirrored to MySQL
// so the web listing shows it without waiting for the Cloud Function.
package ratechange

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	ErrUnavailable = errors.New("rate change is not available for this location")
	ErrNotFound    = errors.New("movement not found")
	ErrNotActive   = errors.New("only active vehicles can change rate")
	ErrNotVehicle  = errors.New("other income tickets cannot change rate")
	ErrInvalidRate = errors.New("the rate is not a current vehicle rate of this location")
	ErrSameRate    = errors.New("the movement already has this rate")
)

// maxObservations is the size of parkmovimientos.observaciones (varchar(200)).
const maxObservations = 200

// Ticket is the part of a stored movement the rate change reads.
type Ticket struct {
	RateCode     int64
	Observations string
	Active       bool // no exit, payment or annulment yet
	OtherIncome  bool // "otros" tickets are not vehicles
}

// Change holds the only fields a rate change writes.
type Change struct {
	RateCode     int64
	UnitPrice    float64
	Observations string
}

// Rate describes a rate code of a location.
type Rate struct {
	Description string
	UnitPrice   float64
}

// Store reads the Firestore configuration and updates tickets atomically.
type Store interface {
	// MovementsDoc returns the document holding the location's movements, or ok=false
	// when the location does not use Firestore.
	MovementsDoc(ctx context.Context, locationID uint) (docID string, ok bool, err error)
	// UpdateTicket reads the ticket and writes the change returned by apply in one transaction.
	// It returns ErrNotFound when the ticket does not exist and any error apply returns.
	UpdateTicket(ctx context.Context, docID, pagoID string, apply func(Ticket) (Change, error)) error
}

// Rates looks up rate codes of a location.
type Rates interface {
	// VehicleRate returns a current vehicle rate of the location, or ok=false.
	VehicleRate(code int, locationID uint) (rate Rate, ok bool, err error)
	// Description returns the description of any rate code of the location, current or not.
	Description(code int, locationID uint) (string, error)
}

// Mirror copies a change already stored in Firestore to the MySQL movement, only while
// the movement is still open there. updated is false when no open row matched, for example
// because the Cloud Function has not inserted the entry yet.
type Mirror interface {
	ApplyRateChange(ctx context.Context, locationID uint, pagoID string, change Change) (updated bool, err error)
}

// Result describes a rate change that was stored in Firestore.
type Result struct {
	MySQLUpdated bool  // the MySQL movement already shows the new rate
	MySQLErr     error // the MySQL copy failed; the Cloud Function sync is the fallback
}

type Request struct {
	LocationID  uint
	PagoID      string
	NewRateCode int
	UserEmail   string
}

type Service struct {
	Store  Store // nil when Firestore is not configured
	Rates  Rates
	Mirror Mirror // nil skips the MySQL copy
	Now    func() time.Time
}

// pagoIDPattern matches the ids the Android app generates and keeps request input from
// addressing another Firestore path.
var pagoIDPattern = regexp.MustCompile(`^[0-9A-Za-z._-]{1,64}$`)

func validPagoID(id string) bool {
	if !pagoIDPattern.MatchString(id) || id == "." || id == ".." {
		return false
	}
	// Firestore reserves ids of the form __name__.
	return !(strings.HasPrefix(id, "__") && strings.HasSuffix(id, "__"))
}

// Available reports whether the location can change rates from the web.
func (s *Service) Available(ctx context.Context, locationID uint) (bool, error) {
	if s.Store == nil {
		return false, nil
	}
	_, ok, err := s.Store.MovementsDoc(ctx, locationID)
	return ok, err
}

// ChangeRate sets a new rate on an active ticket and appends an audit note to its observations.
// Firestore decides: once the change is stored there, it counts as done even if the MySQL copy
// fails, because the device already uses the new rate. Result reports how the copy went.
func (s *Service) ChangeRate(ctx context.Context, req Request) (Result, error) {
	if s.Store == nil {
		return Result{}, ErrUnavailable
	}
	docID, ok, err := s.Store.MovementsDoc(ctx, req.LocationID)
	if err != nil {
		return Result{}, err
	}
	if !ok {
		return Result{}, ErrUnavailable
	}
	if !validPagoID(req.PagoID) {
		return Result{}, ErrNotFound
	}
	newRate, ok, err := s.Rates.VehicleRate(req.NewRateCode, req.LocationID)
	if err != nil {
		return Result{}, err
	}
	if !ok {
		return Result{}, ErrInvalidRate
	}

	// A retried transaction runs apply again; the last run is the one Firestore committed.
	var committed Change
	err = s.Store.UpdateTicket(ctx, docID, req.PagoID, func(ticket Ticket) (Change, error) {
		switch {
		case !ticket.Active:
			return Change{}, ErrNotActive
		case ticket.OtherIncome:
			return Change{}, ErrNotVehicle
		case ticket.RateCode == int64(req.NewRateCode):
			return Change{}, ErrSameRate
		}
		oldDescription, err := s.Rates.Description(int(ticket.RateCode), req.LocationID)
		if err != nil {
			return Change{}, err
		}
		if oldDescription == "" {
			oldDescription = strconv.FormatInt(ticket.RateCode, 10)
		}
		note := fmt.Sprintf("Tarifa cambiada de %s a %s por %s el %s",
			oldDescription, newRate.Description, req.UserEmail, s.Now().Format("2006-01-02 15:04"))
		committed = Change{
			RateCode:     int64(req.NewRateCode),
			UnitPrice:    newRate.UnitPrice,
			Observations: appendNote(ticket.Observations, note),
		}
		return committed, nil
	})
	if err != nil {
		return Result{}, err
	}

	if s.Mirror == nil {
		return Result{}, nil
	}
	updated, err := s.Mirror.ApplyRateChange(ctx, req.LocationID, req.PagoID, committed)
	return Result{MySQLUpdated: updated && err == nil, MySQLErr: err}, nil
}

// appendNote adds note after the existing observations within maxObservations characters.
// The new note always stays whole; older text is trimmed from the start and marked with "...".
func appendNote(existing, note string) string {
	noteRunes := []rune(note)
	if len(noteRunes) >= maxObservations {
		return string(noteRunes[:maxObservations])
	}
	existing = strings.TrimSpace(existing)
	if existing == "" {
		return note
	}
	const separator, ellipsis = " | ", "..."
	room := maxObservations - len(noteRunes) - len(separator)
	old := []rune(existing)
	if len(old) > room {
		if room <= len(ellipsis) {
			return note
		}
		old = append([]rune(ellipsis), old[len(old)-(room-len(ellipsis)):]...)
	}
	return string(old) + separator + note
}
