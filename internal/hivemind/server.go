package hivemind

import (
	"fmt"
	"log"

	"github.com/kiosk404/eidolon/internal/hivemind/config"
	genericapiserver "github.com/kiosk404/eidolon/internal/pkg/server"
	"github.com/kiosk404/eidolon/pkg/http/shutdown"
	"github.com/kiosk404/eidolon/pkg/http/shutdown/posixsignal"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type apiServer struct {
	gs               *shutdown.GracefulShutdown
	gRPCAPIServer    *genericapiserver.GRPCAPIServer
	genericAPIServer *genericapiserver.GenericAPIServer
}

type preparedAPIServer struct {
	*apiServer
}

// ExtraConfig defines extra configuration for the API server.
type ExtraConfig struct {
	Addr       string
	MaxMsgSize int
}

type completedExtraConfig struct {
	*ExtraConfig
}

// Complete fills in any fields not set that are required to have valid data and can be derived from other fields.
func (c *ExtraConfig) complete() *completedExtraConfig {
	if c.Addr == "" {
		c.Addr = "127.0.0.1:11788"
	}

	return &completedExtraConfig{c}
}

// New create a grpcAPIServer instance.
func (c *completedExtraConfig) New() (*genericapiserver.GRPCAPIServer, error) {
	opts := []grpc.ServerOption{grpc.MaxRecvMsgSize(c.MaxMsgSize)}
	grpcServer := grpc.NewServer(opts...)

	reflection.Register(grpcServer)

	return genericapiserver.NewGRPCAPIServer(grpcServer, c.Addr), nil
}

func createAPIServer(cfg *config.Config) (*apiServer, error) {
	gs := shutdown.New()
	gs.AddShutdownManager(posixsignal.NewPosixSignalManager())

	genericConfig, err := buildGenericConfig(cfg)
	if err != nil {
		return nil, err
	}

	extraConfig, err := buildExtraConfig(cfg)
	if err != nil {
		return nil, err
	}

	genericServer, err := genericConfig.Complete().New()
	if err != nil {
		return nil, err
	}
	extraServer, err := extraConfig.complete().New()
	if err != nil {
		return nil, err
	}

	server := &apiServer{
		gs:               gs,
		genericAPIServer: genericServer,
		gRPCAPIServer:    extraServer,
	}

	return server, nil
}

func (s *apiServer) PrepareRun() preparedAPIServer {
	initRouter(s.genericAPIServer.Engine)

	s.gs.AddShutdownCallback(shutdown.Func(func(string) error {

		s.gRPCAPIServer.Stop()
		s.genericAPIServer.Close()
		return nil
	}))
	return preparedAPIServer{s}
}

func (s preparedAPIServer) Run() error {
	go s.gRPCAPIServer.Run()

	// start shutdown managers
	if err := s.gs.Start(); err != nil {
		log.Fatalf("start shutdown manager failed: %s", err.Error())
	}

	return s.genericAPIServer.Run()
}

func buildGenericConfig(cfg *config.Config) (genericConfig *genericapiserver.Config, lastErr error) {
	genericConfig = genericapiserver.NewConfig()
	if lastErr = cfg.GenericServerRunOptions.ApplyTo(genericConfig); lastErr != nil {
		return
	}

	return
}

func buildExtraConfig(cfg *config.Config) (*ExtraConfig, error) {
	return &ExtraConfig{
		Addr:       fmt.Sprintf("%s:%d", cfg.GRPCOptions.BindAddress, cfg.GRPCOptions.BindPort),
		MaxMsgSize: cfg.GRPCOptions.MaxMsgSize,
	}, nil
}
