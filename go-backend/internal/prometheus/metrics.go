package prometheus

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"
)

type MetricSeries struct {
	Metric     string
	DataPoints []DataPoint
}

type ServiceMetrics struct {
	CPUUsage                   MetricSeries
	MemoryUsageMB              MetricSeries
	NetworkReceiveBytesPerSec  MetricSeries
	NetworkTransmitBytesPerSec MetricSeries
}

func (c *Client) GetServiceMetrics(ctx context.Context, namespace, serviceName string, start, end time.Time, step string) (*ServiceMetrics, error) {
	startStr := fmt.Sprintf("%d", start.Unix())
	endStr := fmt.Sprintf("%d", end.Unix())

	cpuQuery := fmt.Sprintf(
		`sum(rate(container_cpu_usage_seconds_total{namespace="%s", pod=~"%s-.*", container!=""}[5m]))`,
		namespace, serviceName,
	)
	memQuery := fmt.Sprintf(
		`sum(container_memory_working_set_bytes{namespace="%s", pod=~"%s-.*", container!=""})`,
		namespace, serviceName,
	)
	netRxQuery := fmt.Sprintf(
		`sum(rate(container_network_receive_bytes_total{namespace="%s", pod=~"%s-.*"}[5m]))`,
		namespace, serviceName,
	)
	netTxQuery := fmt.Sprintf(
		`sum(rate(container_network_transmit_bytes_total{namespace="%s", pod=~"%s-.*"}[5m]))`,
		namespace, serviceName,
	)

	var result ServiceMetrics
	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		points, err := c.QueryRange(gCtx, cpuQuery, startStr, endStr, step)
		if err != nil {
			return fmt.Errorf("cpu query: %w", err)
		}
		result.CPUUsage = MetricSeries{Metric: "cpu_usage", DataPoints: points}
		return nil
	})

	g.Go(func() error {
		points, err := c.QueryRange(gCtx, memQuery, startStr, endStr, step)
		if err != nil {
			return fmt.Errorf("memory query: %w", err)
		}
		for i := range points {
			points[i].Value = points[i].Value / (1024 * 1024)
		}
		result.MemoryUsageMB = MetricSeries{Metric: "memory_usage_mb", DataPoints: points}
		return nil
	})

	g.Go(func() error {
		points, err := c.QueryRange(gCtx, netRxQuery, startStr, endStr, step)
		if err != nil {
			return fmt.Errorf("network rx query: %w", err)
		}
		result.NetworkReceiveBytesPerSec = MetricSeries{Metric: "network_receive_bytes_per_sec", DataPoints: points}
		return nil
	})

	g.Go(func() error {
		points, err := c.QueryRange(gCtx, netTxQuery, startStr, endStr, step)
		if err != nil {
			return fmt.Errorf("network tx query: %w", err)
		}
		result.NetworkTransmitBytesPerSec = MetricSeries{Metric: "network_transmit_bytes_per_sec", DataPoints: points}
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("metrics temporarily unavailable: %w", err)
	}

	return &result, nil
}
