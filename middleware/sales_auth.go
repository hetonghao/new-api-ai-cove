package middleware

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

func SalesAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		authHelper(c, common.RoleSalesUser)
	}
}
