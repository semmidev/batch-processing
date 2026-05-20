package output

import "context"

// SystemCClient defines the output port (driven boundary) for communicating with System C.
type SystemCClient interface {
	SendItem(ctx context.Context, externalID string, payload string) ([]byte, error)
}
