package turso_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/augustdev/autoclip/internal/turso"
)

func getTestConfig() *turso.Config {
	apiKey := os.Getenv("TURSO_APIKEY")
	orgSlug := os.Getenv("TURSO_ORGSLUG")

	if apiKey == "" || orgSlug == "" {
		return nil
	}

	return &turso.Config{
		APIKey:  apiKey,
		OrgSlug: orgSlug,
	}
}

func TestCreateAndDeleteDatabase(t *testing.T) {
	config := getTestConfig()
	if config == nil {
		t.Skip("TURSO_APIKEY and TURSO_ORGSLUG not set")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	client := turso.NewClient(*config, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	dbName := fmt.Sprintf("test-db-%d", time.Now().UnixNano())

	t.Logf("Creating database: %s", dbName)
	db, err := client.CreateDatabase(ctx, &turso.CreateDatabaseRequest{
		Name:      dbName,
		Group:     "eu-central",
		SizeLimit: "100mb",
	})
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	t.Logf("Created: DbID=%s Hostname=%s", db.DbID, db.Hostname)

	if db.DbID == "" {
		t.Error("Expected DbID to be non-empty")
	}
	if db.Hostname == "" {
		t.Error("Expected Hostname to be non-empty")
	}
	if db.Name != dbName {
		t.Errorf("Expected Name=%s, got %s", dbName, db.Name)
	}

	fetchedDB, err := client.GetDatabase(ctx, dbName)
	if err != nil {
		t.Fatalf("Failed to get database: %v", err)
	}
	if fetchedDB.DbID != db.DbID {
		t.Errorf("Expected DbID=%s, got %s", db.DbID, fetchedDB.DbID)
	}

	token, err := client.CreateAuthToken(ctx, dbName, nil)
	if err != nil {
		t.Fatalf("Failed to create auth token: %v", err)
	}
	if token == "" {
		t.Error("Expected token to be non-empty")
	}
	t.Logf("Auth token created (length: %d)", len(token))

	t.Logf("Deleting database: %s", dbName)
	if err := client.DeleteDatabase(ctx, dbName); err != nil {
		t.Fatalf("Failed to delete database: %v", err)
	}

	_, err = client.GetDatabase(ctx, dbName)
	if err == nil {
		t.Error("Expected error when getting deleted database")
	}
}

func TestCreateDatabaseWithReadOnlyToken(t *testing.T) {
	config := getTestConfig()
	if config == nil {
		t.Skip("TURSO_APIKEY and TURSO_ORGSLUG not set")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	client := turso.NewClient(*config, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	dbName := fmt.Sprintf("test-ro-%d", time.Now().UnixNano())

	db, err := client.CreateDatabase(ctx, &turso.CreateDatabaseRequest{
		Name:  dbName,
		Group: "eu-central",
	})
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	defer func() {
		if delErr := client.DeleteDatabase(ctx, dbName); delErr != nil {
			t.Logf("Cleanup failed: %v", delErr)
		}
	}()

	t.Logf("Created: %s (hostname: %s)", db.Name, db.Hostname)

	token, err := client.CreateReadOnlyToken(ctx, dbName, "P7D")
	if err != nil {
		t.Fatalf("Failed to create read-only token: %v", err)
	}
	if token == "" {
		t.Error("Expected token to be non-empty")
	}
	t.Logf("Read-only token created (length: %d)", len(token))
}

func TestListGroups(t *testing.T) {
	config := getTestConfig()
	if config == nil {
		t.Skip("TURSO_APIKEY and TURSO_ORGSLUG not set")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	client := turso.NewClient(*config, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	groups, err := client.ListGroups(ctx)
	if err != nil {
		t.Fatalf("Failed to list groups: %v", err)
	}

	t.Logf("Found %d groups:", len(groups))
	for _, g := range groups {
		t.Logf("  - %s (primary: %s)", g.Name, g.Primary)
	}

	if len(groups) == 0 {
		t.Error("Expected at least one group")
	}

	found := false
	for _, g := range groups {
		if g.Name == "eu-central" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected to find 'eu-central' group")
	}
}

func TestRegionToGroupMapping(t *testing.T) {
	group, ok := turso.RegionToGroup["eu-central"]
	if !ok {
		t.Error("Expected eu-central in RegionToGroup")
	}
	if group != "eu-central" {
		t.Errorf("Expected eu-central to map to eu-central, got %s", group)
	}

	if turso.DefaultRegion != "eu-central" {
		t.Errorf("Expected DefaultRegion=eu-central, got %s", turso.DefaultRegion)
	}

	regions := turso.ValidRegions()
	if len(regions) == 0 {
		t.Error("Expected at least one valid region")
	}

	found := false
	for _, r := range regions {
		if r == "eu-central" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected eu-central in valid regions")
	}
}
