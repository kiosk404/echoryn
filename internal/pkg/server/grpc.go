package server

import (
	"net"

	"github.com/kiosk404/eidolon/pkg/logger"
	"google.golang.org/grpc"
)

type GRPCAPIServer struct {
	*grpc.Server
	address string
}

func NewGRPCAPIServer(srv *grpc.Server, address string) *GRPCAPIServer {
	return &GRPCAPIServer{srv, address}
}

func (s *GRPCAPIServer) Run() {
	listen, err := net.Listen("tcp", s.address)
	if err != nil {
		logger.Fatal("failed to listen: %s", err.Error())
	}

	go func() {
		if err := s.Serve(listen); err != nil {
			logger.Fatal("failed to start grpc server: %s", err.Error())
		}
	}()

	logger.Info("start grpc server at %s", s.address)
}

func (s *GRPCAPIServer) Close() {
	s.GracefulStop()
	logger.Info("GRPC server on %s stopped", s.address)
}
