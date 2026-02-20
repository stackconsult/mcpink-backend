package pg

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/augustdev/autoclip/internal/storage/pg/generated/apikeys"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/clusters"
	deploymentsdb "github.com/augustdev/autoclip/internal/storage/pg/generated/deployments"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/dnsdb"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/githubcreds"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/gittokens"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/internalrepos"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/projects"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/resources"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/services"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/users"
	pgxdecimal "github.com/jackc/pgx-shopspring-decimal"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/fx"
)

type DbConfig struct {
	URL             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}

type DB struct {
	*pgxpool.Pool
	logger          *slog.Logger
	userQueries     users.Querier
	apiKeyQueries   apikeys.Querier
	serviceQueries  services.Querier
	projectQueries  projects.Querier
	githubCredsQ    githubcreds.Querier
	resourceQueries resources.Querier
	internalReposQ  internalrepos.Querier
	gitTokensQ      gittokens.Querier
	dnsQ            dnsdb.Querier
	deploymentsQ    deploymentsdb.Querier
	clustersQ       clusters.Querier
}

func NewDatabase(lc fx.Lifecycle, config DbConfig, logger *slog.Logger) (*DB, error) {
	ctx := context.Background()

	poolConfig, err := pgxpool.ParseConfig(config.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database config: %w", err)
	}

	if config.MaxConns > 0 {
		poolConfig.MaxConns = config.MaxConns
	} else {
		poolConfig.MaxConns = 10
	}

	if config.MinConns > 0 {
		poolConfig.MinConns = config.MinConns
	} else {
		poolConfig.MinConns = 2
	}

	if config.MaxConnLifetime > 0 {
		poolConfig.MaxConnLifetime = config.MaxConnLifetime
	} else {
		poolConfig.MaxConnLifetime = 30 * time.Minute
	}

	if config.MaxConnIdleTime > 0 {
		poolConfig.MaxConnIdleTime = config.MaxConnIdleTime
	} else {
		poolConfig.MaxConnIdleTime = 5 * time.Minute
	}

	logger.Info("Database pool configuration",
		"maxConns", poolConfig.MaxConns,
		"minConns", poolConfig.MinConns,
		"maxConnLifetime", poolConfig.MaxConnLifetime,
		"maxConnIdleTime", poolConfig.MaxConnIdleTime,
	)

	poolConfig.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		pgxdecimal.Register(conn.TypeMap())
		return nil
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	skipPing := envTrue("DB_SKIP_PING")
	skipMigrations := envTrue("DB_SKIP_MIGRATIONS")

	if !skipPing {
		if err := pool.Ping(ctx); err != nil {
			return nil, fmt.Errorf("failed to ping database: %w", err)
		}
		logger.Info("Successfully connected to database")
	} else {
		logger.Warn("Skipping database startup ping (DB_SKIP_PING enabled)")
	}

	if !skipMigrations {
		logger.Info("Running database migrations...")
		if err := RunMigrations(pool); err != nil {
			logger.Error("Failed to run database migrations", "error", err)
			return nil, fmt.Errorf("failed to run database migrations: %w", err)
		}
		logger.Info("Database migrations completed successfully")
	} else {
		logger.Warn("Skipping database migrations on startup (DB_SKIP_MIGRATIONS enabled)")
	}

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			logger.Info("Closing database connection pool...")
			pool.Close()
			logger.Info("Database connection pool closed")
			return nil
		},
	})

	return &DB{
		Pool:            pool,
		logger:          logger,
		userQueries:     users.New(pool),
		apiKeyQueries:   apikeys.New(pool),
		serviceQueries:  services.New(pool),
		projectQueries:  projects.New(pool),
		githubCredsQ:    githubcreds.New(pool),
		resourceQueries: resources.New(pool),
		internalReposQ:  internalrepos.New(pool),
		gitTokensQ:      gittokens.New(pool),
		dnsQ:            dnsdb.New(pool),
		deploymentsQ:    deploymentsdb.New(pool),
		clustersQ:       clusters.New(pool),
	}, nil
}

func envTrue(key string) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func (db *DB) Health() error {
	return db.Ping(context.Background())
}

func NewUserQueries(database *DB) users.Querier {
	return database.userQueries
}

func NewAPIKeyQueries(database *DB) apikeys.Querier {
	return database.apiKeyQueries
}

func NewServiceQueries(database *DB) services.Querier {
	return database.serviceQueries
}

func NewProjectQueries(database *DB) projects.Querier {
	return database.projectQueries
}

func NewGitHubCredsQueries(database *DB) githubcreds.Querier {
	return database.githubCredsQ
}

func NewResourceQueries(database *DB) resources.Querier {
	return database.resourceQueries
}

func NewInternalReposQueries(database *DB) internalrepos.Querier {
	return database.internalReposQ
}

func NewGitTokenQueries(database *DB) gittokens.Querier {
	return database.gitTokensQ
}

func NewDnsQueries(database *DB) dnsdb.Querier {
	return database.dnsQ
}

func NewDeploymentQueries(database *DB) deploymentsdb.Querier {
	return database.deploymentsQ
}

func NewClusterMap(database *DB) (map[string]clusters.Cluster, error) {
	all, err := database.clustersQ.ListClusters(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to load clusters: %w", err)
	}
	m := make(map[string]clusters.Cluster, len(all))
	for _, c := range all {
		m[c.Region] = c
	}
	return m, nil
}
