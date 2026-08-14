package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGetPerformanceStatsExposesHeapRuntimeFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)

	GetPerformanceStats(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			MemoryStats struct {
				HeapAlloc    *uint64 `json:"heap_alloc"`
				HeapIdle     *uint64 `json:"heap_idle"`
				HeapReleased *uint64 `json:"heap_released"`
			} `json:"memory_stats"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.NotNil(t, response.Data.MemoryStats.HeapAlloc)
	require.NotNil(t, response.Data.MemoryStats.HeapIdle)
	require.NotNil(t, response.Data.MemoryStats.HeapReleased)
}
