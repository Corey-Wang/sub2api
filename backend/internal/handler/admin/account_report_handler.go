package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type AccountReportHandler struct{ report *service.AccountReportService }

func NewAccountReportHandler(report *service.AccountReportService) *AccountReportHandler {
	return &AccountReportHandler{report: report}
}
func (h *AccountReportHandler) Get(c *gin.Context) {
	response.Success(c, h.report.Get(c.Request.Context()))
}
