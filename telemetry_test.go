package telemetry

import (
	"context"
	"testing"
)

func TestTracing(t *testing.T) {
	ctx := context.Background()
	EnableOTELTracing(ctx)
}
