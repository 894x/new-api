package controller

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func GetDNSTraffic(c *gin.Context) {
	client, err := service.NewDNSPodTrafficClient()
	if err != nil {
		writeDNSTrafficError(c, err)
		return
	}
	if client == nil {
		common.ApiSuccess(c, service.DNSTrafficState{Records: []service.DNSInstanceRecord{}})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	state, err := client.Read(ctx)
	if err != nil {
		writeDNSTrafficError(c, err)
		return
	}
	common.ApiSuccess(c, state)
}

func GetDNSPodConfig(c *gin.Context) {
	state, err := service.GetDNSPodConfig(c.Request.Context())
	if err != nil {
		writeDNSTrafficError(c, err)
		return
	}
	common.ApiSuccess(c, state)
}

func UpdateDNSPodConfig(c *gin.Context) {
	var update service.DNSPodConfigUpdate
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10))
	if err != nil || common.Unmarshal(body, &update) != nil || service.ValidateDNSPodConfigUpdate(update) != nil {
		writeDNSTrafficError(c, &service.DNSTrafficError{Code: "DNS_CONFIG_INPUT_INVALID"})
		return
	}
	contextJSON, err := common.Marshal(service.DNSPodConfigProofContext{Revision: update.Revision, ChangeHash: service.DNSPodConfigChangeHash(update)})
	if err != nil {
		writeDNSTrafficError(c, &service.DNSTrafficError{Code: "DNS_CONFIG_INPUT_INVALID"})
		return
	}
	if middleware.RequireSecurityProof(c, service.VerificationOperation{Scope: service.VerificationScopeDNSConfigUpdate, Context: contextJSON}) == nil {
		return
	}
	state, err := service.SaveDNSPodConfig(c.Request.Context(), update)
	if err != nil {
		writeDNSTrafficError(c, err)
		return
	}
	common.ApiSuccess(c, state)
}

func UpdateDNSTraffic(c *gin.Context) {
	var update service.DNSTrafficUpdate
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10))
	if err != nil || common.Unmarshal(body, &update) != nil || service.ValidateDNSTrafficUpdate(update) != nil {
		writeDNSTrafficError(c, &service.DNSTrafficError{Code: "DNS_INPUT_INVALID"})
		return
	}
	contextJSON, err := common.Marshal(update)
	if err != nil {
		writeDNSTrafficError(c, &service.DNSTrafficError{Code: "DNS_INPUT_INVALID"})
		return
	}
	if middleware.RequireSecurityProof(c, service.VerificationOperation{Scope: service.VerificationScopeDNSTrafficUpdate, Context: contextJSON}) == nil {
		return
	}
	client, err := service.NewDNSPodTrafficClient()
	if err != nil {
		writeDNSTrafficError(c, err)
		return
	}
	if client == nil {
		writeDNSTrafficError(c, &service.DNSTrafficError{Code: "DNS_CONFIG_INVALID"})
		return
	}
	state, err := client.Apply(c.Request.Context(), update)
	if err != nil {
		writeDNSTrafficError(c, err)
		return
	}
	common.ApiSuccess(c, state)
}

func writeDNSTrafficError(c *gin.Context, err error) {
	failure := &service.DNSTrafficError{Code: "DNS_REQUEST_FAILED"}
	var dnsError *service.DNSTrafficError
	if errors.As(err, &dnsError) {
		failure = dnsError
	}
	status := http.StatusBadGateway
	switch failure.Code {
	case "DNS_INPUT_INVALID", "DNS_CONFIG_INPUT_INVALID":
		status = http.StatusBadRequest
	case "DNS_CONFLICT", "DNS_BUSY", "DNS_CONFIG_CONFLICT":
		status = http.StatusConflict
	case "DNS_CONFIG_INVALID", "DNS_REDIS_REQUIRED", "DNS_ENCRYPTION_KEY_REQUIRED", "DNS_CONFIG_DECRYPT_FAILED", "DNS_CONFIG_STORAGE_FAILED":
		status = http.StatusServiceUnavailable
	case "DNS_RECORDS_INVALID", "DNS_INSTANCE_COUNT_INVALID":
		status = http.StatusUnprocessableEntity
	}
	c.JSON(status, gin.H{"success": false, "message": "DNS traffic update failed. Refresh records before trying again.", "code": failure.Code, "data": failure})
}
