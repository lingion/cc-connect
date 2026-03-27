package core

import "context"

// ObserverTarget is an optional interface that platforms can implement to receive
// terminal observation messages. Currently only Slack implements this.
// Other platforms can implement it in the future without changes to core.
type ObserverTarget interface {
	SendObservation(ctx context.Context, channelID, text string) error
}
