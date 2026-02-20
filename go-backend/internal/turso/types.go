package turso

type CreateDatabaseRequest struct {
	Name      string `json:"name"`
	Group     string `json:"group"`
	SizeLimit string `json:"size_limit,omitempty"`
}

type CreateDatabaseResponse struct {
	Database Database `json:"database"`
}

type Database struct {
	DbID          string   `json:"DbId"`
	Hostname      string   `json:"Hostname"`
	Name          string   `json:"Name"`
	Group         string   `json:"group,omitempty"`
	PrimaryRegion string   `json:"primaryRegion,omitempty"`
	Regions       []string `json:"regions,omitempty"`
	Type          string   `json:"type,omitempty"`
	Version       string   `json:"version,omitempty"`
	IsSchema      bool     `json:"is_schema,omitempty"`
	Schema        string   `json:"schema,omitempty"`
	BlockReads    bool     `json:"block_reads,omitempty"`
	BlockWrites   bool     `json:"block_writes,omitempty"`
	AllowAttach   bool     `json:"allow_attach,omitempty"`
	SizeLimit     string   `json:"size_limit,omitempty"`
	Sleeping      bool     `json:"sleeping,omitempty"`
}

type CreateTokenRequest struct {
	Expiration    string `json:"expiration,omitempty"`    // ISO 8601 duration (e.g., "P30D")
	Authorization string `json:"authorization,omitempty"` // "full-access" or "read-only"
}

type CreateTokenResponse struct {
	JWT string `json:"jwt"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

var RegionToGroup = map[string]string{
	"eu-central": "eu-central",
}

const DefaultRegion = "eu-central"

func ValidRegions() []string {
	regions := make([]string, 0, len(RegionToGroup))
	for region := range RegionToGroup {
		regions = append(regions, region)
	}
	return regions
}
