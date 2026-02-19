package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/augustdev/autoclip/internal/auth"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/projects"
	dbresources "github.com/augustdev/autoclip/internal/storage/pg/generated/resources"
	"github.com/augustdev/autoclip/internal/turso"
	"github.com/lithammer/shortuuid/v4"
)

type Service struct {
	resourcesQ  dbresources.Querier
	projectsQ   projects.Querier
	tursoClient *turso.Client
	authConfig  auth.Config
	logger      *slog.Logger
}

func NewService(
	resourcesQ dbresources.Querier,
	projectsQ projects.Querier,
	tursoClient *turso.Client,
	authConfig auth.Config,
	logger *slog.Logger,
) *Service {
	return &Service{
		resourcesQ:  resourcesQ,
		projectsQ:   projectsQ,
		tursoClient: tursoClient,
		authConfig:  authConfig,
		logger:      logger,
	}
}

func (s *Service) ProvisionDatabase(ctx context.Context, input ProvisionDatabaseInput) (*ProvisionDatabaseOutput, error) {
	if input.Type != TypeSQLite {
		return nil, fmt.Errorf("unsupported database type: %s (only 'sqlite' is supported)", input.Type)
	}

	group, ok := turso.RegionToGroup[input.Region]
	if !ok {
		return nil, fmt.Errorf("unsupported region: %s (supported: %v)", input.Region, turso.ValidRegions())
	}

	if err := validateResourceName(input.Name); err != nil {
		return nil, err
	}

	_, err := s.resourcesQ.GetResourceByUserAndName(ctx, dbresources.GetResourceByUserAndNameParams{
		UserID: input.UserID,
		Name:   input.Name,
	})
	if err == nil {
		return nil, fmt.Errorf("resource with name '%s' already exists", input.Name)
	}

	size := input.Size
	if size == "" {
		size = DefaultSize
	}

	tursoDBName := generateTursoDBName(input.UserID, input.Name)

	s.logger.Info("provisioning database",
		"user_id", input.UserID,
		"name", input.Name,
		"turso_db_name", tursoDBName,
		"type", input.Type,
		"size", size,
		"region", input.Region,
		"group", group,
	)

	db, err := s.tursoClient.CreateDatabase(ctx, &turso.CreateDatabaseRequest{
		Name:      tursoDBName,
		Group:     group,
		SizeLimit: size,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Turso database: %w", err)
	}

	authToken, err := s.tursoClient.CreateAuthToken(ctx, tursoDBName, nil)
	if err != nil {
		_ = s.tursoClient.DeleteDatabase(ctx, tursoDBName)
		return nil, fmt.Errorf("failed to create auth token: %w", err)
	}

	url := fmt.Sprintf("libsql://%s", db.Hostname)

	creds := &Credentials{URL: url, AuthToken: authToken}
	encryptedCreds, err := encryptCredentials(creds, s.authConfig.APIKeyEncryptionKey)
	if err != nil {
		_ = s.tursoClient.DeleteDatabase(ctx, tursoDBName)
		return nil, fmt.Errorf("failed to encrypt credentials: %w", err)
	}

	metadata := Metadata{Size: size, Hostname: db.Hostname, Group: group}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		_ = s.tursoClient.DeleteDatabase(ctx, tursoDBName)
		return nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}

	projectID := ""
	if input.ProjectID != nil {
		projectID = *input.ProjectID
	}
	resource, err := s.resourcesQ.CreateResource(ctx, dbresources.CreateResourceParams{
		ID:          shortuuid.New(),
		UserID:      input.UserID,
		ProjectID:   projectID,
		Name:        input.Name,
		Type:        TypeSQLite,
		Provider:    ProviderTurso,
		Region:      input.Region,
		ExternalID:  &db.DbID,
		Credentials: &encryptedCreds,
		Metadata:    metadataJSON,
		Status:      StatusActive,
	})
	if err != nil {
		_ = s.tursoClient.DeleteDatabase(ctx, tursoDBName)
		return nil, fmt.Errorf("failed to save resource: %w", err)
	}

	s.logger.Info("database provisioned", "resource_id", resource.ID, "name", resource.Name, "url", url)

	return &ProvisionDatabaseOutput{
		ResourceID: resource.ID,
		Name:       resource.Name,
		Type:       resource.Type,
		Region:     resource.Region,
		URL:        url,
		AuthToken:  authToken,
		Status:     resource.Status,
	}, nil
}

func (s *Service) GetResource(ctx context.Context, userID, resourceID string) (*Resource, error) {
	dbResource, err := s.resourcesQ.GetResourceByID(ctx, resourceID)
	if err != nil {
		return nil, fmt.Errorf("resource not found: %w", err)
	}

	if dbResource.UserID != userID {
		return nil, fmt.Errorf("resource not found")
	}

	return s.dbResourceToResource(&dbResource, true)
}

func (s *Service) GetResourceByName(ctx context.Context, userID, name string) (*Resource, error) {
	dbResource, err := s.resourcesQ.GetResourceByUserAndName(ctx, dbresources.GetResourceByUserAndNameParams{
		UserID: userID,
		Name:   name,
	})
	if err != nil {
		return nil, fmt.Errorf("resource not found: %w", err)
	}

	return s.dbResourceToResource(&dbResource, true)
}

func (s *Service) ListResources(ctx context.Context, userID string, limit, offset int32) ([]*Resource, error) {
	dbResources, err := s.resourcesQ.ListResourcesByUser(ctx, dbresources.ListResourcesByUserParams{
		UserID: userID,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list resources: %w", err)
	}

	resources := make([]*Resource, len(dbResources))
	for i, dbr := range dbResources {
		r, err := s.dbResourceToResource(&dbr, false)
		if err != nil {
			return nil, err
		}
		resources[i] = r
	}

	return resources, nil
}

func (s *Service) DeleteResource(ctx context.Context, userID, resourceID string) error {
	resource, err := s.GetResource(ctx, userID, resourceID)
	if err != nil {
		return err
	}

	if resource.Provider == ProviderTurso && resource.ExternalID != nil {
		tursoDBName := generateTursoDBName(userID, resource.Name)
		if err := s.tursoClient.DeleteDatabase(ctx, tursoDBName); err != nil {
			s.logger.Error("failed to delete Turso database", "error", err, "name", tursoDBName)
		}
	}

	if err := s.resourcesQ.DeleteResourceByUserAndID(ctx, dbresources.DeleteResourceByUserAndIDParams{
		ID:     resourceID,
		UserID: userID,
	}); err != nil {
		return fmt.Errorf("failed to delete resource: %w", err)
	}

	s.logger.Info("resource deleted", "resource_id", resourceID)

	return nil
}

func (s *Service) dbResourceToResource(dbr *dbresources.Resource, decryptCreds bool) (*Resource, error) {
	resource := &Resource{
		ID:         dbr.ID,
		UserID:     dbr.UserID,
		ProjectID:  &dbr.ProjectID,
		Name:       dbr.Name,
		Type:       dbr.Type,
		Provider:   dbr.Provider,
		Region:     dbr.Region,
		ExternalID: dbr.ExternalID,
		Status:     dbr.Status,
		CreatedAt:  dbr.CreatedAt.Time,
		UpdatedAt:  dbr.UpdatedAt.Time,
	}

	if dbr.Metadata != nil {
		var metadata map[string]string
		if err := json.Unmarshal(dbr.Metadata, &metadata); err == nil {
			resource.Metadata = metadata
		}
	}

	if decryptCreds && dbr.Credentials != nil && *dbr.Credentials != "" {
		creds, err := decryptCredentials(*dbr.Credentials, s.authConfig.APIKeyEncryptionKey)
		if err != nil {
			s.logger.Error("failed to decrypt credentials", "error", err)
		} else {
			resource.Credentials = creds
		}
	}

	return resource, nil
}

func generateTursoDBName(userID, name string) string {
	prefix := strings.ToLower(userID)
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}

	cleanName := strings.ToLower(name)
	cleanName = regexp.MustCompile(`[^a-z0-9-]`).ReplaceAllString(cleanName, "-")
	cleanName = regexp.MustCompile(`-+`).ReplaceAllString(cleanName, "-")
	cleanName = strings.Trim(cleanName, "-")

	dbName := fmt.Sprintf("%s-%s", prefix, cleanName)

	if len(dbName) < 4 {
		dbName = dbName + "-db"
	}
	if len(dbName) > 64 {
		dbName = dbName[:64]
	}

	return dbName
}

func validateResourceName(name string) error {
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if len(name) < 2 {
		return fmt.Errorf("name must be at least 2 characters")
	}
	if len(name) > 50 {
		return fmt.Errorf("name must be at most 50 characters")
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`).MatchString(name) {
		return fmt.Errorf("name must start with alphanumeric and contain only alphanumeric, hyphens, or underscores")
	}
	return nil
}
