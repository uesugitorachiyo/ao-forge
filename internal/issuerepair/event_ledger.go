package issuerepair

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

type Event struct {
	Sequence   int               `json:"sequence"`
	ID         string            `json:"id"`
	Kind       string            `json:"kind"`
	Fields     map[string]string `json:"fields,omitempty"`
	PrevDigest string            `json:"prev_digest,omitempty"`
	Digest     string            `json:"digest"`
}

type Ledger struct {
	Events []Event `json:"events"`
}

func (ledger Ledger) Has(id string) bool {
	for _, event := range ledger.Events {
		if event.ID == id {
			return true
		}
	}
	return false
}

func (ledger *Ledger) Append(id, kind string, fields map[string]string) error {
	if ledger.Has(id) {
		return ErrDuplicateEvent
	}
	event := Event{
		Sequence: len(ledger.Events) + 1,
		ID:       id,
		Kind:     kind,
		Fields:   fields,
	}
	if len(ledger.Events) > 0 {
		event.PrevDigest = ledger.Events[len(ledger.Events)-1].Digest
	}
	event.Digest = eventDigest(event)
	ledger.Events = append(ledger.Events, event)
	return nil
}

func (ledger Ledger) Head() string {
	if len(ledger.Events) == 0 {
		return ""
	}
	return ledger.Events[len(ledger.Events)-1].Digest
}

func (ledger Ledger) Validate() error {
	seen := make(map[string]struct{}, len(ledger.Events))
	previous := ""
	for index, event := range ledger.Events {
		if event.Sequence != index+1 || event.ID == "" || event.Kind == "" {
			return ErrCheckpointMismatch
		}
		if _, exists := seen[event.ID]; exists {
			return ErrCheckpointMismatch
		}
		if event.PrevDigest != previous || event.Digest != eventDigest(event) {
			return ErrCheckpointMismatch
		}
		seen[event.ID] = struct{}{}
		previous = event.Digest
	}
	return nil
}

func eventDigest(event Event) string {
	payload := struct {
		Sequence   int               `json:"sequence"`
		ID         string            `json:"id"`
		Kind       string            `json:"kind"`
		Fields     map[string]string `json:"fields,omitempty"`
		PrevDigest string            `json:"prev_digest,omitempty"`
	}{
		Sequence:   event.Sequence,
		ID:         event.ID,
		Kind:       event.Kind,
		Fields:     event.Fields,
		PrevDigest: event.PrevDigest,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		panic(fmt.Sprintf("marshal issue-repair event: %v", err))
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
