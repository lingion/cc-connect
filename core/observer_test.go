package core

import (
	"context"
	"testing"
)

func TestObserverTargetInterface(t *testing.T) {
	// Verify the interface exists and has the right method
	var _ ObserverTarget = (*mockObserverTarget)(nil)
}

type mockObserverTarget struct{}

func (m *mockObserverTarget) SendObservation(ctx context.Context, channelID, text string) error {
	return nil
}
