package githubapp

type Config struct {
	AppID              int64  `mapstructure:"appid"`
	PrivateKey         string `mapstructure:"privatekey"`
	ClientID           string `mapstructure:"clientid"`
	ClientSecret       string `mapstructure:"clientsecret"`
	CoolifyPrivKeyUUID string `mapstructure:"coolifyprivkeyuuid"`
}
