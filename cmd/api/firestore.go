package main

import (
	"context"
	"log"
	"time"

	"github.com/parqueo/api/internal/firestoredb"
	"github.com/parqueo/api/internal/ratechange"
	"github.com/parqueo/api/internal/repository"
)

// newRateChanger connects to Firestore when FIREBASE_PROJECT_ID is set. Without it, or when
// the connection fails, the API still starts and the rate change reports itself unavailable.
func newRateChanger() *ratechange.Service {
	svc := &ratechange.Service{Rates: repository.RateCatalog{}, Now: time.Now}
	store, err := firestoredb.Connect(context.Background())
	switch {
	case err != nil:
		log.Printf("Firestore disabled: %v", err)
	case store == nil:
		log.Println("Firestore disabled: FIREBASE_PROJECT_ID is not set")
	default:
		svc.Store = store
	}
	return svc
}
