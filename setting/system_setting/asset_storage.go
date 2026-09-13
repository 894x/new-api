package system_setting

import (
	"errors"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// AssetStorageConfig is deployment-owned. Credentials never enter the option API.
type AssetStorageConfig struct {
	Enabled      bool
	Bucket       string
	Region       string
	SecretID     string
	SecretKey    string
	SessionToken string
	URLLifetime  time.Duration
}

func LoadAssetStorageConfig() (AssetStorageConfig, error) {
	c := AssetStorageConfig{Enabled: os.Getenv("ASSET_STORAGE_ENABLED") == "true", Bucket: strings.TrimSpace(os.Getenv("COS_BUCKET")), Region: strings.TrimSpace(os.Getenv("COS_REGION")), SecretID: os.Getenv("COS_SECRET_ID"), SecretKey: os.Getenv("COS_SECRET_KEY"), SessionToken: os.Getenv("COS_SESSION_TOKEN"), URLLifetime: 24 * time.Hour}
	if value := os.Getenv("ASSET_STORAGE_URL_TTL_SECONDS"); value != "" {
		seconds, err := strconv.Atoi(value)
		if err != nil || seconds < 60 || seconds > 7*24*3600 {
			return c, errors.New("ASSET_STORAGE_URL_TTL_SECONDS must be between 60 and 604800")
		}
		c.URLLifetime = time.Duration(seconds) * time.Second
	}
	if c.Enabled && (!regexp.MustCompile(`^[a-z0-9][a-z0-9-]*-[0-9]+$`).MatchString(c.Bucket) || !regexp.MustCompile(`^[a-z]+-[a-z0-9-]+$`).MatchString(c.Region) || c.SecretID == "" || c.SecretKey == "") {
		return c, errors.New("asset storage requires a valid COS_BUCKET, COS_REGION, COS_SECRET_ID and COS_SECRET_KEY")
	}
	return c, nil
}

func (c AssetStorageConfig) Endpoint() *url.URL {
	return &url.URL{Scheme: "https", Host: c.Bucket + ".cos." + c.Region + ".myqcloud.com"}
}
