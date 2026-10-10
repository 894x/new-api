package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// The entire configuration occupies one existing options row. Credentials never
// enter OptionMap or the generic settings API; all instances read this row live.
type DNSPodConfigUpdate struct {
	Revision          string `json:"revision"`
	Enabled           bool   `json:"enabled"`
	Domain            string `json:"domain"`
	Subdomain         string `json:"subdomain"`
	RecordLine        string `json:"record_line"`
	RecordType        string `json:"record_type"`
	SecretID          string `json:"secret_id"`
	SecretKey         string `json:"secret_key"`
	SessionToken      string `json:"session_token"`
	ClearSessionToken bool   `json:"clear_session_token"`
}

type DNSPodConfigState struct {
	Revision        string `json:"revision"`
	Enabled         bool   `json:"enabled"`
	Domain          string `json:"domain"`
	Subdomain       string `json:"subdomain"`
	RecordLine      string `json:"record_line"`
	RecordType      string `json:"record_type"`
	SecretIDSet     bool   `json:"secret_id_set"`
	SecretKeySet    bool   `json:"secret_key_set"`
	SessionTokenSet bool   `json:"session_token_set"`
	Source          string `json:"source"`
	CanSave         bool   `json:"can_save"`
	BlockedReason   string `json:"blocked_reason,omitempty"`
}

type storedDNSPodConfig struct {
	Enabled bool `json:"enabled"`
	DNSPodTrafficConfig
}

type DNSPodConfigProofContext struct {
	Revision   string `json:"revision"`
	ChangeHash string `json:"change_hash"`
}

// Use the deployment's stable server secret with a domain-separated key. Never
// encrypt persistent data using the random, per-process default SessionSecret.
func dnsConfigCipher() (cipher.AEAD, error) {
	secret := os.Getenv("CRYPTO_SECRET")
	if secret == "" {
		secret = os.Getenv("SESSION_SECRET")
	}
	if len(secret) < 32 || secret == "random_string" {
		return nil, &DNSTrafficError{Code: "DNS_ENCRYPTION_KEY_REQUIRED"}
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("new-api/dnspod-config/v1"))
	block, err := aes.NewCipher(mac.Sum(nil))
	if err != nil {
		return nil, &DNSTrafficError{Code: "DNS_CONFIG_STORAGE_FAILED"}
	}
	return cipher.NewGCMWithRandomNonce(block)
}

func loadDNSPodConfig(ctx context.Context) (storedDNSPodConfig, string, string, error) {
	var option model.Option
	if model.DB == nil {
		return storedDNSPodConfig{}, "", "", &DNSTrafficError{Code: "DNS_CONFIG_STORAGE_FAILED"}
	}
	err := model.DB.WithContext(ctx).Where(&model.Option{Key: model.DNSPodConfigOptionKey}).First(&option).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		config := storedDNSPodConfig{DNSPodTrafficConfig: DNSPodTrafficConfig{
			SecretID: os.Getenv("DNSPOD_SECRET_ID"), SecretKey: os.Getenv("DNSPOD_SECRET_KEY"), SessionToken: os.Getenv("DNSPOD_SESSION_TOKEN"),
			Domain: strings.ToLower(strings.TrimSpace(os.Getenv("DNSPOD_DOMAIN"))), Subdomain: strings.ToLower(strings.TrimSpace(os.Getenv("DNSPOD_SUBDOMAIN"))),
			RecordLine: strings.TrimSpace(os.Getenv("DNSPOD_RECORD_LINE")), RecordType: strings.ToUpper(strings.TrimSpace(os.Getenv("DNSPOD_RECORD_TYPE"))),
		}}
		if config.RecordLine == "" {
			config.RecordLine = "默认"
		}
		if config.RecordType == "" {
			config.RecordType = "A"
		}
		config.Enabled = config.Domain != "" || config.Subdomain != "" || config.SecretID != "" || config.SecretKey != "" || config.SessionToken != ""
		return config, "", "environment", nil
	}
	if err != nil {
		return storedDNSPodConfig{}, "", "", &DNSTrafficError{Code: "DNS_CONFIG_STORAGE_FAILED"}
	}
	aead, err := dnsConfigCipher()
	if err != nil {
		return storedDNSPodConfig{}, "", "", err
	}
	ciphertext, err := hex.DecodeString(strings.TrimPrefix(option.Value, "v1:"))
	if !strings.HasPrefix(option.Value, "v1:") || err != nil {
		return storedDNSPodConfig{}, "", "", &DNSTrafficError{Code: "DNS_CONFIG_DECRYPT_FAILED"}
	}
	plaintext, err := aead.Open(nil, nil, ciphertext, []byte(model.DNSPodConfigOptionKey))
	var config storedDNSPodConfig
	if err != nil || common.Unmarshal(plaintext, &config) != nil {
		return storedDNSPodConfig{}, "", "", &DNSTrafficError{Code: "DNS_CONFIG_DECRYPT_FAILED"}
	}
	return config, option.Value, "database", nil
}

func dnsPodConfigRevision(config storedDNSPodConfig, raw string) string {
	if raw == "" {
		encoded, _ := common.Marshal(config)
		// The environment fallback revision must not expose a credential digest.
		return common.GenerateHMACWithKey(authSigningKey("dns-config-revision"), string(encoded))
	}
	digest := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(digest[:])
}

func GetDNSPodConfig(ctx context.Context) (*DNSPodConfigState, error) {
	config, raw, source, err := loadDNSPodConfig(ctx)
	if err != nil {
		return nil, err
	}
	state := &DNSPodConfigState{
		Revision: dnsPodConfigRevision(config, raw), Enabled: config.Enabled,
		Domain: config.Domain, Subdomain: config.Subdomain, RecordLine: config.RecordLine, RecordType: config.RecordType,
		SecretIDSet: config.SecretID != "", SecretKeySet: config.SecretKey != "", SessionTokenSet: config.SessionToken != "", Source: source,
		CanSave: true,
	}
	if _, err := dnsConfigCipher(); err != nil {
		state.CanSave, state.BlockedReason = false, "DNS_ENCRYPTION_KEY_REQUIRED"
	} else if !common.RedisEnabled || common.RDB == nil {
		state.CanSave, state.BlockedReason = false, "DNS_REDIS_REQUIRED"
	}
	return state, nil
}

func ValidateDNSPodConfigUpdate(update DNSPodConfigUpdate) error {
	revision, err := hex.DecodeString(update.Revision)
	if err != nil || len(revision) != sha256.Size || len(update.Domain) > 253 || len(update.Subdomain) > 253 || len(update.RecordLine) > 128 || len(update.SecretID) > 256 || len(update.SecretKey) > 256 || len(update.SessionToken) > 8192 || (update.ClearSessionToken && update.SessionToken != "") {
		return &DNSTrafficError{Code: "DNS_CONFIG_INPUT_INVALID"}
	}
	for _, value := range []string{update.SecretID, update.SecretKey, update.SessionToken, update.RecordLine} {
		if strings.TrimSpace(value) != value || strings.ContainsAny(value, "\r\n\x00") {
			return &DNSTrafficError{Code: "DNS_CONFIG_INPUT_INVALID"}
		}
	}
	if update.RecordType != "A" && update.RecordType != "AAAA" {
		return &DNSTrafficError{Code: "DNS_CONFIG_INPUT_INVALID"}
	}
	// Empty fields are useful when disabling an unconfigured installation.
	if update.Domain == "" && update.Subdomain == "" && !update.Enabled {
		return nil
	}
	for index, name := range []string{update.Domain, update.Subdomain} {
		if index == 1 && name == "@" {
			continue
		}
		if name == "" || (index == 0 && !strings.Contains(name, ".")) {
			return &DNSTrafficError{Code: "DNS_CONFIG_INPUT_INVALID"}
		}
		for label := range strings.SplitSeq(name, ".") {
			if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
				return &DNSTrafficError{Code: "DNS_CONFIG_INPUT_INVALID"}
			}
			for _, ch := range label {
				if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-') {
					return &DNSTrafficError{Code: "DNS_CONFIG_INPUT_INVALID"}
				}
			}
		}
	}
	if update.RecordLine == "" || (update.Subdomain != "@" && len(update.Domain)+len(update.Subdomain)+1 > 253) {
		return &DNSTrafficError{Code: "DNS_CONFIG_INPUT_INVALID"}
	}
	return nil
}

// Length-prefixed UTF-8 fields have the same canonical encoding in Go and the
// browser, including non-ASCII line names. Proof flows receive only this digest.
func DNSPodConfigChangeHash(update DNSPodConfigUpdate) string {
	var canonical strings.Builder
	for _, value := range []string{update.Revision, strconv.FormatBool(update.Enabled), update.Domain, update.Subdomain, update.RecordLine, update.RecordType, update.SecretID, update.SecretKey, update.SessionToken, strconv.FormatBool(update.ClearSessionToken)} {
		canonical.WriteString(strconv.Itoa(len(value)))
		canonical.WriteByte(':')
		canonical.WriteString(value)
	}
	digest := sha256.Sum256([]byte(canonical.String()))
	return hex.EncodeToString(digest[:])
}

// Configuration changes and provider writes share a lock, including across
// different DNS scopes, so a pending update cannot outlive a configuration edit.
func acquireDNSManagementLock(ctx context.Context) (string, error) {
	if !common.RedisEnabled || common.RDB == nil {
		return "", &DNSTrafficError{Code: "DNS_REDIS_REQUIRED"}
	}
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", &DNSTrafficError{Code: "DNS_REQUEST_FAILED"}
	}
	token := hex.EncodeToString(tokenBytes)
	acquired, err := common.RDB.SetNX(ctx, "dns-management:lock", token, 150*time.Second).Result()
	if err != nil {
		return "", &DNSTrafficError{Code: "DNS_REDIS_REQUIRED"}
	}
	if !acquired {
		return "", &DNSTrafficError{Code: "DNS_BUSY"}
	}
	return token, nil
}

func releaseDNSManagementLock(token string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = redis.NewScript("if redis.call('get', KEYS[1]) == ARGV[1] then return redis.call('del', KEYS[1]) else return 0 end").Run(ctx, common.RDB, []string{"dns-management:lock"}, token).Err()
}

func SaveDNSPodConfig(ctx context.Context, update DNSPodConfigUpdate) (*DNSPodConfigState, error) {
	if err := ValidateDNSPodConfigUpdate(update); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	token, err := acquireDNSManagementLock(ctx)
	if err != nil {
		return nil, err
	}
	keepLease := false
	defer func() {
		if !keepLease {
			releaseDNSManagementLock(token)
		}
	}()
	config, raw, _, err := loadDNSPodConfig(ctx)
	if err != nil {
		return nil, err
	}
	if update.Revision != dnsPodConfigRevision(config, raw) {
		return nil, &DNSTrafficError{Code: "DNS_CONFIG_CONFLICT"}
	}
	config.Enabled = update.Enabled
	config.Domain, config.Subdomain = strings.ToLower(update.Domain), strings.ToLower(update.Subdomain)
	config.RecordLine, config.RecordType = update.RecordLine, update.RecordType
	// Blank secret inputs preserve the effective value, including the initial
	// environment configuration. Disabling is persisted and never falls back.
	if update.SecretID != "" {
		config.SecretID = update.SecretID
	}
	if update.SecretKey != "" {
		config.SecretKey = update.SecretKey
	}
	if update.ClearSessionToken {
		config.SessionToken = ""
	} else if update.SessionToken != "" {
		config.SessionToken = update.SessionToken
	}
	if config.Enabled && (config.SecretID == "" || config.SecretKey == "") {
		return nil, &DNSTrafficError{Code: "DNS_CONFIG_INPUT_INVALID"}
	}
	aead, err := dnsConfigCipher()
	if err != nil {
		return nil, err
	}
	plaintext, err := common.Marshal(config)
	if err != nil {
		return nil, &DNSTrafficError{Code: "DNS_CONFIG_STORAGE_FAILED"}
	}
	encoded := "v1:" + hex.EncodeToString(aead.Seal(nil, nil, plaintext, []byte(model.DNSPodConfigOptionKey)))
	owner, err := common.RDB.Get(ctx, "dns-management:lock").Result()
	if err != nil || owner != token {
		return nil, &DNSTrafficError{Code: "DNS_BUSY"}
	}
	var result *gorm.DB
	if raw == "" {
		result = model.DB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&model.Option{Key: model.DNSPodConfigOptionKey, Value: encoded})
	} else {
		result = model.DB.WithContext(ctx).Model(&model.Option{}).Where(&model.Option{Key: model.DNSPodConfigOptionKey}).Where("value = ?", raw).Update("value", encoded)
	}
	if result.Error != nil {
		// A database timeout can leave the commit result uncertain. Give the
		// server time to settle before another management operation is started.
		keepLease = true
		return nil, &DNSTrafficError{Code: "DNS_CONFIG_STORAGE_FAILED"}
	}
	if result.RowsAffected != 1 {
		return nil, &DNSTrafficError{Code: "DNS_CONFIG_CONFLICT"}
	}
	return GetDNSPodConfig(ctx)
}
