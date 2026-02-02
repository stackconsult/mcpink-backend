package resources

import "time"

type Resource struct {
	ID          string            `json:"id"`
	UserID      string            `json:"user_id"`
	ProjectID   *string           `json:"project_id,omitempty"`
	Name        string            `json:"name"`
	Type        string            `json:"type"`
	Provider    string            `json:"provider"`
	Region      string            `json:"region"`
	ExternalID  *string           `json:"external_id,omitempty"`
	Credentials *Credentials      `json:"credentials,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Status      string            `json:"status"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

type Credentials struct {
	URL       string `json:"url"`
	AuthToken string `json:"auth_token"`
}

type Metadata struct {
	Size     string `json:"size,omitempty"`
	Hostname string `json:"hostname,omitempty"`
	Group    string `json:"group,omitempty"`
}

type ProvisionDatabaseInput struct {
	UserID    string
	ProjectID *string
	Name      string
	Type      string
	Size      string
	Region    string
}

type ProvisionDatabaseOutput struct {
	ResourceID string `json:"resource_id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Region     string `json:"region"`
	URL        string `json:"url"`
	AuthToken  string `json:"auth_token"`
	Status     string `json:"status"`
}

const (
	TypeSQLite   = "sqlite"
	TypePostgres = "postgres"
	TypeMongo    = "mongo"
	TypeLLM      = "llm"
)

const (
	ProviderTurso  = "turso"
	ProviderNeon   = "neon"
	ProviderAtlas  = "atlas"
	ProviderOpenAI = "openai"
)

const (
	StatusProvisioning = "provisioning"
	StatusActive       = "active"
	StatusFailed       = "failed"
	StatusDeleting     = "deleting"
	StatusDeleted      = "deleted"
)

const (
	DefaultSize   = "100mb"
	DefaultRegion = "eu-west"
)
