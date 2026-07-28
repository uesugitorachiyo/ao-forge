package issuerepair

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

type Lease struct {
	WorkerID  string    `json:"worker_id"`
	Token     string    `json:"token"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

func newLease(workerID string, now time.Time, ttl time.Duration) (Lease, error) {
	var tokenBytes [16]byte
	if _, err := rand.Read(tokenBytes[:]); err != nil {
		return Lease{}, fmt.Errorf("create issue-repair lease token: %w", err)
	}
	return Lease{
		WorkerID:  workerID,
		Token:     hex.EncodeToString(tokenBytes[:]),
		IssuedAt:  now,
		ExpiresAt: now.Add(ttl),
	}, nil
}

func renewLease(current Lease, workerID string, now time.Time, ttl time.Duration) (Lease, error) {
	if current.WorkerID != workerID && now.Before(current.ExpiresAt) {
		return Lease{}, ErrLeaseConflict
	}
	return newLease(workerID, now, ttl)
}
