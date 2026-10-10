package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const dnsTrafficMaxInstances = 16

// Isolate credential transport from the relay's optional TLS bypass setting.
// This shared transport retains connection pooling across management requests.
var dnsPodTransport = &http.Transport{
	Proxy:               http.ProxyFromEnvironment,
	DialContext:         (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
	TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
	ForceAttemptHTTP2:   true,
	TLSHandshakeTimeout: 10 * time.Second,
	IdleConnTimeout:     90 * time.Second,
}

// The dedicated root configuration endpoint owns credentials and DNS scope.
// Weight updates accept only existing record IDs and weights.
type DNSPodTrafficConfig struct {
	SecretID, SecretKey, SessionToken         string
	Domain, Subdomain, RecordLine, RecordType string
}

type DNSInstanceRecord struct {
	ID        string `json:"id"`
	IP        string `json:"ip"`
	Weight    int    `json:"weight"`
	TTL       int    `json:"ttl"`
	Enabled   bool   `json:"enabled"`
	Remark    string `json:"remark"`
	UpdatedOn string `json:"updated_on"`
}

type DNSTrafficState struct {
	Configured    bool                `json:"configured"`
	CanApply      bool                `json:"can_apply"`
	BlockedReason string              `json:"blocked_reason,omitempty"`
	Domain        string              `json:"domain"`
	Subdomain     string              `json:"subdomain"`
	RecordLine    string              `json:"record_line"`
	RecordType    string              `json:"record_type"`
	Revision      string              `json:"revision"`
	Records       []DNSInstanceRecord `json:"records"`
}

type DNSInstanceWeight struct {
	ID     string `json:"id"`
	Weight int    `json:"weight"`
}

type DNSTrafficUpdate struct {
	Revision string              `json:"revision"`
	Weights  []DNSInstanceWeight `json:"weights"`
}

// DNS errors expose only stable codes and a provider request ID, never a raw
// transport error, signature, credentials or the provider's free-form message.
type DNSTrafficError struct {
	Code           string `json:"code"`
	ProviderCode   string `json:"provider_code,omitempty"`
	RequestID      string `json:"request_id,omitempty"`
	MayHaveChanged bool   `json:"may_have_changed"`
}

func (e *DNSTrafficError) Error() string { return e.Code }

func NewDNSPodTrafficClient() (*DNSPodTrafficClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stored, raw, _, err := loadDNSPodConfig(ctx)
	if err != nil {
		return nil, err
	}
	if !stored.Enabled {
		return nil, nil
	}
	config := stored.DNSPodTrafficConfig
	if config.SecretID == "" || config.SecretKey == "" || config.Domain == "" || config.Subdomain == "" || (config.RecordType != "A" && config.RecordType != "AAAA") || strings.ContainsAny(config.Domain+config.Subdomain, " /\\\r\n\t") {
		return nil, &DNSTrafficError{Code: "DNS_CONFIG_INVALID"}
	}
	return &DNSPodTrafficClient{config: config, configRevision: dnsPodConfigRevision(stored, raw), httpClient: &http.Client{
		Timeout:       10 * time.Second,
		Transport:     dnsPodTransport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

type DNSPodTrafficClient struct {
	config         DNSPodTrafficConfig
	httpClient     *http.Client
	configRevision string
}

// tencentAuthorization implements Tencent Cloud TC3 with the action, content
// type and host signed. The timestamp's date MUST be UTC.
func tencentAuthorization(secretID, secretKey, host, service, action string, body []byte, timestamp int64) string {
	date := time.Unix(timestamp, 0).UTC().Format("2006-01-02")
	payloadHash := sha256.Sum256(body)
	canonical := "POST\n/\n\ncontent-type:application/json; charset=utf-8\nhost:" + host + "\nx-tc-action:" + strings.ToLower(action) + "\n\ncontent-type;host;x-tc-action\n" + hex.EncodeToString(payloadHash[:])
	canonicalHash := sha256.Sum256([]byte(canonical))
	scope := date + "/" + service + "/tc3_request"
	toSign := "TC3-HMAC-SHA256\n" + strconv.FormatInt(timestamp, 10) + "\n" + scope + "\n" + hex.EncodeToString(canonicalHash[:])
	key := []byte("TC3" + secretKey)
	for _, value := range []string{date, service, "tc3_request", toSign} {
		mac := hmac.New(sha256.New, key)
		_, _ = mac.Write([]byte(value))
		key = mac.Sum(nil)
	}
	return "TC3-HMAC-SHA256 Credential=" + secretID + "/" + scope + ", SignedHeaders=content-type;host;x-tc-action, Signature=" + hex.EncodeToString(key)
}

func (client *DNSPodTrafficClient) call(ctx context.Context, action string, payload any, result any) error {
	body, err := common.Marshal(payload)
	if err != nil {
		return &DNSTrafficError{Code: "DNS_REQUEST_FAILED"}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://dnspod.tencentcloudapi.com/", bytes.NewReader(body))
	if err != nil {
		return &DNSTrafficError{Code: "DNS_REQUEST_FAILED"}
	}
	timestamp := time.Now().Unix()
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	request.Header.Set("X-TC-Action", action)
	request.Header.Set("X-TC-Version", "2021-03-23")
	request.Header.Set("X-TC-Timestamp", strconv.FormatInt(timestamp, 10))
	request.Header.Set("Authorization", tencentAuthorization(client.config.SecretID, client.config.SecretKey, request.URL.Host, "dnspod", action, body, timestamp))
	if client.config.SessionToken != "" {
		request.Header.Set("X-TC-Token", client.config.SessionToken)
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		return &DNSTrafficError{Code: "DNS_REQUEST_FAILED"}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return &DNSTrafficError{Code: "DNS_REQUEST_FAILED"}
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
	if err != nil || len(data) > 2<<20 {
		return &DNSTrafficError{Code: "DNS_RESPONSE_INVALID"}
	}
	var envelope struct{ Response common.RawMessage }
	if common.Unmarshal(data, &envelope) != nil || common.GetJsonType(envelope.Response) != "object" {
		return &DNSTrafficError{Code: "DNS_RESPONSE_INVALID"}
	}
	var status struct {
		Error     *struct{ Code string }
		RequestID string `json:"RequestId"`
	}
	if common.Unmarshal(envelope.Response, &status) != nil || status.RequestID == "" {
		return &DNSTrafficError{Code: "DNS_RESPONSE_INVALID"}
	}
	if status.Error != nil {
		// Request IDs and error codes are identifiers; reject unexpected content.
		for _, value := range []string{status.Error.Code, status.RequestID} {
			if len(value) > 128 || strings.ContainsFunc(value, func(r rune) bool {
				return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_')
			}) {
				return &DNSTrafficError{Code: "DNS_PROVIDER_ERROR"}
			}
		}
		return &DNSTrafficError{Code: "DNS_PROVIDER_ERROR", ProviderCode: status.Error.Code, RequestID: status.RequestID}
	}
	if common.Unmarshal(envelope.Response, result) != nil {
		return &DNSTrafficError{Code: "DNS_RESPONSE_INVALID"}
	}
	return nil
}

func (client *DNSPodTrafficClient) Read(ctx context.Context) (*DNSTrafficState, error) {
	state := &DNSTrafficState{Configured: true, Domain: client.config.Domain, Subdomain: client.config.Subdomain, RecordLine: client.config.RecordLine, RecordType: client.config.RecordType, Records: []DNSInstanceRecord{}}
	seen := map[int64]bool{}
	for offset := 0; ; {
		var result struct {
			RecordList []struct {
				RecordID                                           int64 `json:"RecordId"`
				Value, Name, Type, Line, Status, Remark, UpdatedOn string
				Weight                                             *int
				TTL                                                int
			}
		}
		err := client.call(ctx, "DescribeRecordList", map[string]any{"Domain": client.config.Domain, "SubDomain": client.config.Subdomain, "RecordLine": client.config.RecordLine, "RecordType": client.config.RecordType, "Offset": offset, "Limit": 100, "ErrorOnEmpty": "no"}, &result)
		if err != nil {
			return nil, err
		}
		for _, record := range result.RecordList {
			if record.RecordID <= 0 || seen[record.RecordID] {
				return nil, &DNSTrafficError{Code: "DNS_RESPONSE_INVALID"}
			}
			seen[record.RecordID] = true
			if !strings.EqualFold(record.Name, client.config.Subdomain) || record.Type != client.config.RecordType || record.Line != client.config.RecordLine {
				continue
			}
			weight := 0
			if record.Weight != nil {
				weight = *record.Weight
			}
			if weight < 0 || weight > 100 || record.TTL <= 0 || (record.Status != "ENABLE" && record.Status != "DISABLE") {
				return nil, &DNSTrafficError{Code: "DNS_RESPONSE_INVALID"}
			}
			state.Records = append(state.Records, DNSInstanceRecord{ID: strconv.FormatInt(record.RecordID, 10), IP: record.Value, Weight: weight, TTL: record.TTL, Enabled: record.Status == "ENABLE", Remark: record.Remark, UpdatedOn: record.UpdatedOn})
		}
		offset += len(result.RecordList)
		if len(result.RecordList) < 100 {
			break
		}
		if offset >= 2000 {
			return nil, &DNSTrafficError{Code: "DNS_TOO_MANY_RECORDS"}
		}
	}
	slices.SortFunc(state.Records, func(a, b DNSInstanceRecord) int { return strings.Compare(a.ID, b.ID) })
	payload, err := common.Marshal(state)
	if err != nil {
		return nil, &DNSTrafficError{Code: "DNS_RESPONSE_INVALID"}
	}
	hash := sha256.Sum256(append([]byte(client.configRevision+"\x00"), payload...))
	state.Revision = hex.EncodeToString(hash[:])
	enabled := 0
	ips := map[string]bool{}
	for _, record := range state.Records {
		if !record.Enabled {
			continue
		}
		enabled++
		ip, err := netip.ParseAddr(record.IP)
		if err != nil || ip.Zone() != "" || (client.config.RecordType == "A" && !ip.Is4()) || (client.config.RecordType == "AAAA" && (!ip.Is6() || ip.Is4In6())) || ips[ip.String()] {
			state.BlockedReason = "DNS_RECORDS_INVALID"
		}
		ips[ip.String()] = true
	}
	if state.BlockedReason == "" && (enabled < 2 || enabled > dnsTrafficMaxInstances) {
		state.BlockedReason = "DNS_INSTANCE_COUNT_INVALID"
	}
	if state.BlockedReason == "" && (!common.RedisEnabled || common.RDB == nil) {
		state.BlockedReason = "DNS_REDIS_REQUIRED"
	}
	state.CanApply = state.BlockedReason == ""
	return state, nil
}

func ValidateDNSTrafficUpdate(update DNSTrafficUpdate) error {
	revision, err := hex.DecodeString(update.Revision)
	if err != nil || len(revision) != sha256.Size || len(update.Weights) < 2 || len(update.Weights) > dnsTrafficMaxInstances {
		return &DNSTrafficError{Code: "DNS_INPUT_INVALID"}
	}
	seen := map[string]bool{}
	for _, weight := range update.Weights {
		id, err := strconv.ParseInt(weight.ID, 10, 64)
		if err != nil || id <= 0 || strconv.FormatInt(id, 10) != weight.ID || seen[weight.ID] || weight.Weight < 1 || weight.Weight > 100 {
			return &DNSTrafficError{Code: "DNS_INPUT_INVALID"}
		}
		seen[weight.ID] = true
	}
	return nil
}

func (client *DNSPodTrafficClient) Apply(ctx context.Context, update DNSTrafficUpdate) (*DNSTrafficState, error) {
	if err := ValidateDNSTrafficUpdate(update); err != nil {
		return nil, err
	}
	if !common.RedisEnabled || common.RDB == nil {
		return nil, &DNSTrafficError{Code: "DNS_REDIS_REQUIRED"}
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	token, err := acquireDNSManagementLock(ctx)
	if err != nil {
		return nil, err
	}
	keepLease := false
	defer func() {
		// An uncertain/partial provider write retains the lease until expiry;
		// callers must refresh and reconcile, not automatically retry a write.
		if keepLease {
			return
		}
		releaseDNSManagementLock(token)
	}()
	if client.configRevision != "" {
		config, raw, _, err := loadDNSPodConfig(ctx)
		if err != nil {
			return nil, err
		}
		if dnsPodConfigRevision(config, raw) != client.configRevision {
			return nil, &DNSTrafficError{Code: "DNS_CONFIG_CONFLICT"}
		}
	}
	current, err := client.Read(ctx)
	if err != nil {
		return nil, err
	}
	if current.Revision != update.Revision {
		return nil, &DNSTrafficError{Code: "DNS_CONFLICT"}
	}
	if !current.CanApply {
		return nil, &DNSTrafficError{Code: current.BlockedReason}
	}
	desired := map[string]int{}
	for _, weight := range update.Weights {
		desired[weight.ID] = weight.Weight
	}
	enabled := 0
	for _, record := range current.Records {
		if record.Enabled {
			enabled++
			if _, ok := desired[record.ID]; !ok {
				return nil, &DNSTrafficError{Code: "DNS_INPUT_INVALID"}
			}
		}
	}
	if enabled != len(desired) {
		return nil, &DNSTrafficError{Code: "DNS_INPUT_INVALID"}
	}
	for _, record := range current.Records {
		if !record.Enabled || record.Weight == desired[record.ID] {
			continue
		}
		owner, err := common.RDB.Get(ctx, "dns-management:lock").Result()
		if err != nil || owner != token {
			return nil, &DNSTrafficError{Code: "DNS_BUSY", MayHaveChanged: keepLease}
		}
		keepLease = true
		id, _ := strconv.ParseInt(record.ID, 10, 64)
		var result struct {
			RecordID int64 `json:"RecordId"`
		}
		err = client.call(ctx, "ModifyRecordFields", map[string]any{"Domain": client.config.Domain, "RecordId": id, "FieldList": []map[string]string{{"Key": "weight", "Value": strconv.Itoa(desired[record.ID])}}}, &result)
		if err != nil {
			var failure *DNSTrafficError
			if errors.As(err, &failure) {
				failure.MayHaveChanged = true
				return nil, failure
			}
			return nil, &DNSTrafficError{Code: "DNS_REQUEST_FAILED", MayHaveChanged: true}
		}
		if result.RecordID != id {
			return nil, &DNSTrafficError{Code: "DNS_RESPONSE_INVALID", MayHaveChanged: true}
		}
	}
	verified, err := client.Read(ctx)
	if err != nil {
		return nil, &DNSTrafficError{Code: "DNS_VERIFY_FAILED", MayHaveChanged: keepLease}
	}
	// The complete managed snapshot must match, allowing only weight and
	// provider-maintained update timestamps to change.
	if len(verified.Records) != len(current.Records) {
		return nil, &DNSTrafficError{Code: "DNS_VERIFY_FAILED", MayHaveChanged: keepLease}
	}
	for i, record := range verified.Records {
		expected := current.Records[i]
		if expected.Enabled {
			expected.Weight = desired[expected.ID]
		}
		expected.UpdatedOn = record.UpdatedOn
		if record != expected {
			return nil, &DNSTrafficError{Code: "DNS_VERIFY_FAILED", MayHaveChanged: keepLease}
		}
	}
	keepLease = false
	return verified, nil
}
