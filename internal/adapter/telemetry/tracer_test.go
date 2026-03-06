package telemetry_test

import (
	"context"
	"testing"

	"github.com/asakaida/dandori/internal/adapter/telemetry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitTracer_NoEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	tracer, shutdown, err := telemetry.InitTracer(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, tracer)
	assert.NotNil(t, shutdown)

	err = shutdown(context.Background())
	assert.NoError(t, err)
}
