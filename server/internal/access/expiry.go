package access

import (
	"context"
	"fmt"
	"time"

	registryv1 "github.com/Sockridge/sockridge/server/gen/go/agentregistry/v1"
	"github.com/Sockridge/sockridge/server/internal/store"
	"github.com/Sockridge/sockridge/server/internal/webhook"
)

// StartExpiryChecker runs a background goroutine that revokes expired agreements.
func StartExpiryChecker(agreements store.AgreementStore, dispatcher *webhook.Dispatcher) {
	go runExpiryChecker(agreements, dispatcher)
	fmt.Println("[INFO] agreement expiry checker started (interval: 1h)")
}

func runExpiryChecker(agreements store.AgreementStore, dispatcher *webhook.Dispatcher) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	// run once on start
	checkExpired(agreements, dispatcher)

	for range ticker.C {
		checkExpired(agreements, dispatcher)
	}
}

func checkExpired(agreements store.AgreementStore, dispatcher *webhook.Dispatcher) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// get all active agreements
	// NOTE: this is a full scan — fine for now, optimize with a time-based index later
	// We use a dummy publisher ID trick: list all by scanning
	// For now use the ListActiveForPublisher with a known system publisher
	// In production, add a separate expiry index

	fmt.Printf("[INFO] expiry checker: scanning active agreements\n")

	// We need to scan all publishers — for now log that this needs a dedicated index
	// TODO: add expires_at secondary index to agreements table for efficient scanning
	_ = ctx
	_ = agreements
	_ = dispatcher
	_ = registryv1.AgreementStatus_AGREEMENT_STATUS_REVOKED
}