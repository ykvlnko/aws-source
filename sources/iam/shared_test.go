package iam

import (
	"context"
	"log"
	"os"
	"testing"

	"github.com/overmindtech/aws-source/sources"
	"github.com/overmindtech/aws-source/tracing"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
)

// TestIAMClient Test client that returns three pages
type TestIAMClient struct{}

var TestRateLimit = sources.LimitBucket{
	MaxCapacity: 50,
	RefillRate:  20,
}

func TestMain(m *testing.M) {
	// Add tracing if present
	key, _ := os.LookupEnv("HONEYCOMB_API_KEY")
	opts := make([]otlptracehttp.Option, 0)
	if key != "" {
		opts = []otlptracehttp.Option{
			otlptracehttp.WithEndpoint("api.honeycomb.io"),
			otlptracehttp.WithHeaders(map[string]string{"x-honeycomb-team": key}),
		}
	}

	if err := tracing.InitTracing(opts...); err != nil {
		log.Fatal(err)
	}
	defer tracing.ShutdownTracing()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	TestRateLimit.Start(ctx)

	os.Exit(m.Run())
}
