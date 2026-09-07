package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func GetChannelModelMatrix(c *gin.Context) {
	filter := model.ChannelModelMatrixFilter{
		Status: "enabled", ModelPage: 1, ModelPageSize: 25, ChannelPage: 1, ChannelPageSize: 10,
	}
	if err := c.ShouldBindQuery(&filter); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	matrix, err := model.ListChannelModelMatrix(filter)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, matrix)
}
