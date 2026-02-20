package statuschecker

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"k8s.io/client-go/kubernetes"
)

type Config struct {
	Port           string
	Interval       string
	InstatusAPIKey string
	InstatusPageID string
	Checks         []CheckConfig
}

type CheckConfig struct {
	Name          string
	Type          string // "http", "k8s_pod", "dns", "k8s_nodes"
	URL           string
	LabelSelector string
	Namespace     string
	Server        string
	Domain        string
	ComponentID   string
}

type Checker struct {
	cfg      Config
	k8s      kubernetes.Interface
	instatus *InstatusClient
	logger   *slog.Logger

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func New(cfg Config, k8s kubernetes.Interface, logger *slog.Logger) *Checker {
	return &Checker{
		cfg:      cfg,
		k8s:      k8s,
		instatus: NewInstatusClient(cfg.InstatusPageID, cfg.InstatusAPIKey),
		logger:   logger,
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

	// Run immediately on start
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

func (c *Checker) runChecks(ctx context.Context) {
	for _, check := range c.cfg.Checks {
		if check.ComponentID == "" {
			continue
		}

		err := c.executeCheck(ctx, check)
		status := StatusOperational
		if err != nil {
			status = StatusMajorOutage
			c.logger.Error("Check failed", "name", check.Name, "type", check.Type, "error", err)
		} else {
			c.logger.Debug("Check passed", "name", check.Name, "type", check.Type)
		}

		if err := c.instatus.UpdateComponentStatus(ctx, check.ComponentID, status); err != nil {
			c.logger.Error("Failed to update Instatus", "name", check.Name, "component", check.ComponentID, "error", err)
		}
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
