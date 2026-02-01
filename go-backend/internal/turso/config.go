package turso

type Config struct {
	APIKey  string `mapstructure:"apikey"`
	OrgSlug string `mapstructure:"orgslug"`
}
