package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func ListRoutingServiceTypes(c *gin.Context) {
	rows, err := model.ListRoutingServiceTypes()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	common.ApiSuccess(c, rows)
}

func SaveRoutingServiceType(c *gin.Context) {
	var row model.RoutingServiceType
	if common.DecodeJson(http.MaxBytesReader(c.Writer, c.Request.Body, 8<<10), &row) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid service type"})
		return
	}
	if err := model.SaveRoutingServiceType(&row, row.Revision); err != nil {
		routingServiceTypeError(c, err)
		return
	}
	common.ApiSuccess(c, row)
}

func DeleteRoutingServiceType(c *gin.Context) {
	var row model.RoutingServiceType
	if common.DecodeJson(http.MaxBytesReader(c.Writer, c.Request.Body, 8<<10), &row) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid service type"})
		return
	}
	if err := model.DeleteRoutingServiceType(row.ID, row.Revision); err != nil {
		routingServiceTypeError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func GetUserRoutingServiceTypes(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("id"))
	if err != nil || userID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid user"})
		return
	}
	bindings, err := model.ListUserRoutingServiceTypes(userID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	types, err := model.ListRoutingServiceTypes()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	common.ApiSuccess(c, gin.H{"bindings": bindings, "service_types": types})
}

func SaveUserRoutingServiceType(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("id"))
	var request struct {
		model.UserRoutingServiceType
		Reset bool `json:"reset"`
	}
	if err != nil || userID <= 0 || common.DecodeJson(http.MaxBytesReader(c.Writer, c.Request.Body, 8<<10), &request) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid service type binding"})
		return
	}
	request.UserID = userID
	if err := model.SaveUserRoutingServiceType(&request.UserRoutingServiceType, request.Revision, request.Reset); err != nil {
		routingServiceTypeError(c, err)
		return
	}
	recordManageAuditFor(c, userID, "user.routing_service_type.update", map[string]any{"model": request.Model, "routing_group": request.RoutingGroup, "service_type_id": request.ServiceTypeID, "reset": request.Reset})
	common.ApiSuccess(c, request.UserRoutingServiceType)
}

func routingServiceTypeError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	if errors.Is(err, model.ErrInvalidRoutingServiceType) || errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, gorm.ErrDuplicatedKey) {
		status = http.StatusBadRequest
	}
	if errors.Is(err, model.ErrRoutingPoolConflict) || errors.Is(err, model.ErrRoutingServiceTypeInUse) || errors.Is(err, model.ErrRoutingServiceTypeNameExists) {
		status = http.StatusConflict
	}
	c.JSON(status, gin.H{"success": false, "message": err.Error()})
}
