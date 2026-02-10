package bootstrap

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"time"

	"github.com/augustdev/autoclip/internal/k8sdeployments"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/contrib/opentelemetry"
	"go.temporal.io/sdk/interceptor"
	"go.temporal.io/sdk/worker"
	"go.uber.org/fx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
)

type TemporalClientConfig struct {
	Address     string
	Namespace   string
	CloudAPIKey string
}

func CreateTemporalClient(lc fx.Lifecycle, config TemporalClientConfig) (client.Client, error) {
	tracingInterceptor, err := opentelemetry.NewTracingInterceptor(opentelemetry.TracerOptions{})
	if err != nil {
		slog.Warn("Failed to create OpenTelemetry tracing interceptor, continuing without tracing",
			"error", err)
	}

	clientOptions := client.Options{
		HostPort:  config.Address,
		Namespace: config.Namespace,
		ConnectionOptions: client.ConnectionOptions{
			DialOptions: []grpc.DialOption{
				grpc.WithKeepaliveParams(keepalive.ClientParameters{
					Time:                5 * time.Minute,
					Timeout:             20 * time.Second,
					PermitWithoutStream: true,
				}),
				grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
				grpc.WithUnaryInterceptor(
					func(ctx context.Context, method string, req any, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
						slog.Debug("Temporal gRPC call",
							"method", method,
							"hasDeadline", func() bool { _, ok := ctx.Deadline(); return ok }())

						ctx = metadata.AppendToOutgoingContext(ctx, config.Namespace, config.Namespace)

						return invoker(
							ctx,
							method,
							req,
							reply,
							cc,
							opts...,
						)
					},
				),
				grpc.WithDefaultCallOptions(
					grpc.MaxCallRecvMsgSize(32*1024*1024),
					grpc.MaxCallSendMsgSize(32*1024*1024),
				),
			},
		},
	}

	if config.CloudAPIKey != "" {
		clientOptions.ConnectionOptions.TLS = &tls.Config{}
		clientOptions.Credentials = client.NewAPIKeyStaticCredentials(config.CloudAPIKey)
	}

	if tracingInterceptor != nil {
		clientOptions.Interceptors = []interceptor.ClientInterceptor{tracingInterceptor}
	}

	slog.Info("Creating lazy Temporal client (connection deferred until first use)",
		"address", config.Address,
		"namespace", config.Namespace,
		"cloudAuth", config.CloudAPIKey != "")

	c, err := client.NewLazyClient(clientOptions)
	if err != nil {
		slog.Error("Failed to create Temporal lazy client",
			"address", config.Address,
			"error", err)
		return nil, fmt.Errorf("failed to create temporal lazy client: %w", err)
	}

	slog.Info("Successfully created lazy Temporal client - now validating connection",
		"address", config.Address,
		"namespace", config.Namespace)

	healthCheckTimeout := 90 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), healthCheckTimeout)
	defer cancel()

	deadline, hasDeadline := ctx.Deadline()
	slog.Info("Starting Temporal health check with fresh context",
		"timeout", healthCheckTimeout,
		"hasDeadline", hasDeadline,
		"deadline", deadline,
		"timeUntilDeadline", time.Until(deadline))

	healthCheckStart := time.Now()
	_, err = c.WorkflowService().GetSystemInfo(ctx, nil)
	healthCheckDuration := time.Since(healthCheckStart)

	if err != nil {
		slog.Error("Temporal health check failed - blocking startup",
			"address", config.Address,
			"namespace", config.Namespace,
			"error", err,
			"duration", healthCheckDuration,
			"contextErr", ctx.Err(),
			"deadlineExceeded", ctx.Err() == context.DeadlineExceeded,
			"canceled", ctx.Err() == context.Canceled)

		c.Close()
		return nil, fmt.Errorf("temporal health check failed after %v: %w", healthCheckDuration, err)
	}

	slog.Info("Temporal health check succeeded - client validated and ready",
		"address", config.Address,
		"namespace", config.Namespace,
		"duration", healthCheckDuration)

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			slog.Info("Closing Temporal client...")
			c.Close()
			slog.Info("Temporal client closed")
			return nil
		},
	})

	return c, nil
}

func NewTemporalWorker(c client.Client) worker.Worker {
	w := worker.New(c, "default", worker.Options{
		WorkerStopTimeout: 10 * time.Minute,
	})
	return w
}

func NewK8sTemporalWorker(c client.Client) worker.Worker {
	w := worker.New(c, k8sdeployments.TaskQueue, worker.Options{
		WorkerStopTimeout: 10 * time.Minute,
	})
	return w
}
