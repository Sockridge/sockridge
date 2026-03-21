package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	registryv1 "github.com/Sockridge/sockridge/server/gen/go/agentregistry/v1"
)

// Event types
const (
	EventAccessRequest  = "access_request"
	EventAccessApproved = "access_approved"
	EventAccessDenied   = "access_denied"
	EventAccessRevoked  = "access_revoked"
	EventAgentActive    = "agent_active"
	EventAgentInactive  = "agent_inactive"
	EventAgentPublished = "agent_published"
	EventAgentRejected  = "agent_rejected"
)

type Payload struct {
	Event       string                 `json:"event"`
	PublisherID string                 `json:"publisher_id"`
	OccurredAt  string                 `json:"occurred_at"`
	Data        map[string]interface{} `json:"data"`
}

type Dispatcher struct {
	store  *Store
	client *http.Client
}

func NewDispatcher(store *Store) *Dispatcher {
	return &Dispatcher{
		store: store,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Fire sends a webhook event to all registered URLs for a publisher.
// Runs async — never blocks the main request.
func (d *Dispatcher) Fire(publisherID, event string, data map[string]interface{}) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		webhooks, err := d.store.ListActiveByEvent(ctx, publisherID, event)
		if err != nil || len(webhooks) == 0 {
			return
		}

		payload := Payload{
			Event:       event,
			PublisherID: publisherID,
			OccurredAt:  time.Now().UTC().Format(time.RFC3339),
			Data:        data,
		}

		body, err := json.Marshal(payload)
		if err != nil {
			return
		}

		for _, wh := range webhooks {
			d.deliver(wh, body)
		}
	}()
}

func (d *Dispatcher) deliver(wh *registryv1.Webhook, body []byte) {
	sig := sign(body, wh.Secret)

	req, err := http.NewRequest(http.MethodPost, wh.Url, bytes.NewReader(body))
	if err != nil {
		fmt.Printf("[WARN] webhook: invalid URL %s: %v\n", wh.Url, err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Sockridge-Event", wh.Events[0])
	req.Header.Set("X-Sockridge-Signature", "sha256="+sig)
	req.Header.Set("X-Sockridge-Delivery", wh.Id)

	resp, err := d.client.Do(req)
	if err != nil {
		fmt.Printf("[WARN] webhook: delivery failed to %s: %v\n", wh.Url, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		fmt.Printf("[INFO] webhook: delivered %s to %s (%d)\n", wh.Events[0], wh.Url, resp.StatusCode)
	} else {
		fmt.Printf("[WARN] webhook: delivery to %s returned %d\n", wh.Url, resp.StatusCode)
	}
}

func sign(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}