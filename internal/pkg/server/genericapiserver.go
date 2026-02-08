package server

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"github.com/kiosk404/eidolon/internal/pkg/core"
	"github.com/kiosk404/eidolon/pkg/logger"
	"github.com/kiosk404/eidolon/pkg/version"
)

// GenericAPIServer contains state for a generic api server.
// type GenericAPIServer gin.Engine.
type GenericAPIServer struct {
	middlewares []string
	// ServingInfo holds configuration of the TLS server.
	ServingInfo *ServingInfo

	// ShutdownTimeout is the timeout used for server shutdown. This specifies the timeout before server
	// gracefully shutdown returns.
	ShutdownTimeout time.Duration

	*gin.Engine
	healthz         bool
	enableMetrics   bool
	enableProfiling bool

	Server *http.Server
}

func initGenericAPIServer(s *GenericAPIServer) {
	// do some setup
	// s.GET(path, ginSwagger.WrapHandler(swaggerFiles.Handler))

	s.Setup()
	s.InstallMiddlewares()
	s.InstallAPIs()
}

// Setup do some setup work for gin engine.
func (s *GenericAPIServer) Setup() {
	gin.DebugPrintRouteFunc = func(httpMethod, absolutePath, handlerName string, nuHandlers int) {
		logger.Info("%-6s %-s --> %s (%d handlers)", httpMethod, absolutePath, handlerName, nuHandlers)
	}
}

// InstallMiddlewares installs middlewares to gin engine.
func (s *GenericAPIServer) InstallMiddlewares() {

}

func (s *GenericAPIServer) InstallAPIs() {
	// install healthz handler
	if s.healthz {
		s.GET("/healthz", func(c *gin.Context) {
			core.WriteResponse(c, nil, map[string]string{"status": "ok"})
		})
	}

	// install pprof handler
	if s.enableProfiling {
		pprof.Register(s.Engine)
	}

	s.GET("/version", func(c *gin.Context) {
		core.WriteResponse(c, nil, version.Get())
	})
}

// Close graceful shutdown the api server.
func (s *GenericAPIServer) Close() {
	// The context is used to inform the server it has 10 seconds to finish
	// the request it is currently handling
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := s.Server.Shutdown(ctx); err != nil {
		logger.Warn("Shutdown secure server failed: %s", err.Error())
	}
}
