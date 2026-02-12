package graph

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/augustdev/autoclip/internal/graph/model"
	"github.com/augustdev/autoclip/internal/prometheus"
)

type timeRangeParams struct {
	Duration time.Duration
	Step     string
}

var timeRangeMap = map[model.MetricTimeRange]timeRangeParams{
	model.MetricTimeRangeOneHour:    {Duration: 1 * time.Hour, Step: "15s"},
	model.MetricTimeRangeSixHours:   {Duration: 6 * time.Hour, Step: "1m"},
	model.MetricTimeRangeSevenDays:  {Duration: 7 * 24 * time.Hour, Step: "5m"},
	model.MetricTimeRangeThirtyDays: {Duration: 30 * 24 * time.Hour, Step: "30m"},
}

func resolveTimeRange(tr model.MetricTimeRange) (start, end time.Time, step string, err error) {
	params, ok := timeRangeMap[tr]
	if !ok {
		return time.Time{}, time.Time{}, "", fmt.Errorf("invalid time range: %s", tr)
	}
	now := time.Now()
	return now.Add(-params.Duration), now, params.Step, nil
}

func toModelSeries(s prometheus.MetricSeries) *model.MetricSeries {
	points := make([]*model.MetricDataPoint, len(s.DataPoints))
	for i, dp := range s.DataPoints {
		points[i] = &model.MetricDataPoint{
			Timestamp: dp.Timestamp,
			Value:     dp.Value,
		}
	}
	return &model.MetricSeries{
		Metric:     s.Metric,
		DataPoints: points,
	}
}

// parseMemoryToMB parses memory strings like "512Mi", "1Gi", "256" into MB.
func parseMemoryToMB(memory string) float64 {
	memory = strings.TrimSpace(memory)
	if memory == "" {
		return 0
	}

	if strings.HasSuffix(memory, "Gi") {
		val, err := strconv.ParseFloat(strings.TrimSuffix(memory, "Gi"), 64)
		if err != nil {
			return 0
		}
		return val * 1024
	}

	if strings.HasSuffix(memory, "Mi") {
		val, err := strconv.ParseFloat(strings.TrimSuffix(memory, "Mi"), 64)
		if err != nil {
			return 0
		}
		return val
	}

	val, err := strconv.ParseFloat(memory, 64)
	if err != nil {
		return 0
	}
	return val
}

// parseCPUToVCPUs parses CPU strings like "100m", "0.5", "1" into vCPUs.
func parseCPUToVCPUs(cpu string) float64 {
	cpu = strings.TrimSpace(cpu)
	if cpu == "" {
		return 0
	}

	if strings.HasSuffix(cpu, "m") {
		val, err := strconv.ParseFloat(strings.TrimSuffix(cpu, "m"), 64)
		if err != nil {
			return 0
		}
		return val / 1000
	}

	val, err := strconv.ParseFloat(cpu, 64)
	if err != nil {
		return 0
	}
	return val
}
