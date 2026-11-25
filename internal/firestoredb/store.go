// Package firestoredb connects to the Firestore database the Android app writes to.
package firestoredb

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"cloud.google.com/go/firestore"
	"github.com/parqueo/api/internal/ratechange"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	rootCollection      = "Parqueos RN"
	configDocument      = "reqfirestore"
	movementsCollection = "movimientos"

	fieldRateCode     = "codigoPresupuestario"
	fieldUnitPrice    = "precioUnitario"
	fieldObservations = "observaciones"
)

// Store implements ratechange.Store on Firestore.
type Store struct {
	client *firestore.Client
}

// Connect opens a Firestore client for FIREBASE_PROJECT_ID using Application Default
// Credentials (GOOGLE_APPLICATION_CREDENTIALS pointing to a service account file).
// It returns nil, nil when FIREBASE_PROJECT_ID is unset, so the API runs without Firestore.
func Connect(ctx context.Context) (*Store, error) {
	projectID := strings.TrimSpace(os.Getenv("FIREBASE_PROJECT_ID"))
	if projectID == "" {
		return nil, nil
	}
	client, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("connect to Firestore project %q: %w", projectID, err)
	}
	return &Store{client: client}, nil
}

func (s *Store) Close() error {
	return s.client.Close()
}

// MovementsDoc reads Parqueos RN/reqfirestore and returns the document that holds the
// location's movements ("{prefix}{id}", e.g. "LT-8").
func (s *Store) MovementsDoc(ctx context.Context, locationID uint) (string, bool, error) {
	snap, err := s.client.Collection(rootCollection).Doc(configDocument).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read Firestore config: %w", err)
	}
	doc, ok := movementsDocFromConfig(snap.Data(), locationID)
	return doc, ok, nil
}

// UpdateTicket runs apply on the current ticket and writes the rate, unit price and
// observations in one transaction, so a concurrent exit cannot slip in between.
func (s *Store) UpdateTicket(ctx context.Context, docID, pagoID string, apply func(ratechange.Ticket) (ratechange.Change, error)) error {
	ref := s.client.Collection(rootCollection).Doc(docID).Collection(movementsCollection).Doc(pagoID)
	return s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(ref)
		if status.Code(err) == codes.NotFound {
			return ratechange.ErrNotFound
		}
		if err != nil {
			return err
		}
		change, err := apply(ticketFromData(snap.Data()))
		if err != nil {
			return err
		}
		return tx.Update(ref, []firestore.Update{
			{Path: fieldRateCode, Value: change.RateCode},
			{Path: fieldUnitPrice, Value: change.UnitPrice},
			{Path: fieldObservations, Value: change.Observations},
		})
	})
}

// movementsDocFromConfig finds the location in "ubicaciones" (ids stored as strings) and
// builds its document id from the prefix map in "prefijos"[0].
func movementsDocFromConfig(data map[string]any, locationID uint) (string, bool) {
	id := strconv.FormatUint(uint64(locationID), 10)

	locations, _ := data["ubicaciones"].([]any)
	connected := false
	for _, location := range locations {
		if idString(location) == id {
			connected = true
			break
		}
	}
	if !connected {
		return "", false
	}

	prefixes, _ := data["prefijos"].([]any)
	if len(prefixes) == 0 {
		return "", false
	}
	prefixMap, _ := prefixes[0].(map[string]any)
	prefix, ok := prefixMap[id].(string)
	if !ok {
		return "", false
	}
	return prefix + id, true
}

func idString(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return ""
}

func ticketFromData(data map[string]any) ratechange.Ticket {
	var code int64
	switch v := data[fieldRateCode].(type) {
	case int64:
		code = v
	case float64:
		code = int64(v)
	}
	observations, _ := data[fieldObservations].(string)
	plate, _ := data["placa"].(string)
	return ratechange.Ticket{
		RateCode:     code,
		Observations: observations,
		Active:       unset(data["fechaHorasale"]) && unset(data["fechaHorapago"]) && unset(data["fechaHoraAnula"]),
		OtherIncome:  strings.EqualFold(strings.TrimSpace(plate), "otros"),
	}
}

// unset treats a missing, null or empty date as not set.
func unset(value any) bool {
	if value == nil {
		return true
	}
	s, ok := value.(string)
	return ok && strings.TrimSpace(s) == ""
}

var _ ratechange.Store = (*Store)(nil)
