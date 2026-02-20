package statuschecker

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"k8s.io/client-go/kubernetes"
)

type Config struct {
	Port     string
	Interval string
	Checks   []CheckConfig
}

type CheckConfig struct {
	Name          string
	Type          string // "http", "k8s_pod", "dns", "k8s_nodes"
	URL           string
	LabelSelector string
	Namespace     string
	Server        string
	Domain        string
}

type CheckResult struct {
	Healthy bool   `json:"healthy"`
	Error   string `json:"error,omitempty"`
}

type Checker struct {
	cfg    Config
	k8s    kubernetes.Interface
	logger *slog.Logger

	mu      sync.RWMutex
	results map[string]CheckResult

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func New(cfg Config, k8s kubernetes.Interface, logger *slog.Logger) *Checker {
	return &Checker{
		cfg:     cfg,
		k8s:     k8s,
		logger:  logger,
		results: make(map[string]CheckResult),
	}
}

func (c *Checker) Run(parentCtx context.Context) {
	interval, err := time.ParseDuration(c.cfg.Interval)
	if err != nil {
		interval = 30 * time.Second
		c.logger.Warn("Invalid interval, using default", "configured", c.cfg.Interval, "default", interval)
	}

	ctx, cancel := context.WithCancel(parentCtx)
	c.cancel = cancel

	c.wg.Add(1)
	defer c.wg.Done()

	c.runChecks(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.runChecks(ctx)
		}
	}
}

func (c *Checker) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
	c.wg.Wait()
}

func (c *Checker) GetResult(name string) (CheckResult, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	r, ok := c.results[name]
	return r, ok
}

func (c *Checker) GetAllResults() map[string]CheckResult {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]CheckResult, len(c.results))
	for k, v := range c.results {
		out[k] = v
	}
	return out
}

func (c *Checker) runChecks(ctx context.Context) {
	for _, check := range c.cfg.Checks {
		err := c.executeCheck(ctx, check)

		result := CheckResult{Healthy: true}
		if err != nil {
			result = CheckResult{Healthy: false, Error: err.Error()}
			c.logger.Error("Check failed", "name", check.Name, "type", check.Type, "error", err)
		} else {
			c.logger.Debug("Check passed", "name", check.Name, "type", check.Type)
		}

		c.mu.Lock()
		c.results[check.Name] = result
		c.mu.Unlock()
	}
}

func (c *Checker) executeCheck(ctx context.Context, check CheckConfig) error {
	switch check.Type {
	case "http":
		return httpCheck(check.URL, 5*time.Second)
	case "k8s_pod":
		ns := check.Namespace
		if ns == "" {
			ns = "dp-system"
		}
		return k8sPodCheck(ctx, c.k8s, ns, check.LabelSelector)
	case "dns":
		return dnsCheck(check.Server, check.Domain)
	case "k8s_nodes":
		return k8sNodesCheck(ctx, c.k8s)
	default:
		c.logger.Warn("Unknown check type", "type", check.Type, "name", check.Name)
		return nil
	}
}
