package prometheus

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"
)

type TimeRange string

const (
	TimeRangeOneHour    TimeRange = "ONE_HOUR"
	TimeRangeSixHours   TimeRange = "SIX_HOURS"
	TimeRangeSevenDays  TimeRange = "SEVEN_DAYS"
	TimeRangeThirtyDays TimeRange = "THIRTY_DAYS"
)

type timeRangeParams struct {
	Duration time.Duration
	Step     string
}

var timeRangeMap = map[TimeRange]timeRangeParams{
	TimeRangeOneHour:    {Duration: 1 * time.Hour, Step: "15s"},
	TimeRangeSixHours:   {Duration: 6 * time.Hour, Step: "1m"},
	TimeRangeSevenDays:  {Duration: 7 * 24 * time.Hour, Step: "5m"},
	TimeRangeThirtyDays: {Duration: 30 * 24 * time.Hour, Step: "30m"},
}

type MetricSeries struct {
	Metric     string
	DataPoints []DataPoint
}

type AppMetrics struct {
	CPUUsage                   MetricSeries
	MemoryUsageMB              MetricSeries
	NetworkReceiveBytesPerSec  MetricSeries
	NetworkTransmitBytesPerSec MetricSeries
}

func (c *Client) GetAppMetrics(ctx context.Context, namespace, serviceName string, tr TimeRange) (*AppMetrics, error) {
	params, ok := timeRangeMap[tr]
	if !ok {
		return nil, fmt.Errorf("invalid time range: %s", tr)
	}

	now := time.Now()
	start := fmt.Sprintf("%d", now.Add(-params.Duration).Unix())
	end := fmt.Sprintf("%d", now.Unix())

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

	var result AppMetrics
	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		points, err := c.QueryRange(gCtx, cpuQuery, start, end, params.Step)
		if err != nil {
			return fmt.Errorf("cpu query: %w", err)
		}
		result.CPUUsage = MetricSeries{Metric: "cpu_usage", DataPoints: points}
		return nil
	})

	g.Go(func() error {
		points, err := c.QueryRange(gCtx, memQuery, start, end, params.Step)
		if err != nil {
			return fmt.Errorf("memory query: %w", err)
		}
		// Convert bytes to MB
		for i := range points {
			points[i].Value = points[i].Value / (1024 * 1024)
		}
		result.MemoryUsageMB = MetricSeries{Metric: "memory_usage_mb", DataPoints: points}
		return nil
	})

	g.Go(func() error {
		points, err := c.QueryRange(gCtx, netRxQuery, start, end, params.Step)
		if err != nil {
			return fmt.Errorf("network rx query: %w", err)
		}
		result.NetworkReceiveBytesPerSec = MetricSeries{Metric: "network_receive_bytes_per_sec", DataPoints: points}
		return nil
	})

	g.Go(func() error {
		points, err := c.QueryRange(gCtx, netTxQuery, start, end, params.Step)
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
