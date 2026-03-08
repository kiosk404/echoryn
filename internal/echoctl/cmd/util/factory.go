package util

import (
	"fmt"
	"net/http"

	pb "github.com/kiosk404/echoryn/pkg/proto/golem"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Factory provides abstractions that allow the echoctl command to be extended across multiple types
// of resources and different API sets.
type Factory interface {
	HTTPClient() *http.Client
	// AdminClient returns a HivemindAdminService gRPC client connected to the given address.
	// The caller is responsible for closing the returned connection.
	AdminClient(addr string) (pb.HivemindAdminServiceClient, *grpc.ClientConn, error)
	// HivemindAddr returns the configured Hivemind gRPC address.
	HivemindAddr() string
}

type defaultFactory struct {
	hivemindAddr *string
}

// NewDefaultFactory creates a Factory. The hivemindAddr pointer should point to the
// global flag variable that holds the Hivemind address.
func NewDefaultFactory(hivemindAddr *string) Factory {
	return &defaultFactory{hivemindAddr: hivemindAddr}
}

func (f *defaultFactory) HTTPClient() *http.Client {
	return http.DefaultClient
}

func (f *defaultFactory) HivemindAddr() string {
	if f.hivemindAddr != nil {
		return *f.hivemindAddr
	}
	return "127.0.0.1:11788"
}

func (f *defaultFactory) AdminClient(addr string) (pb.HivemindAdminServiceClient, *grpc.ClientConn, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to Hivemind at %s: %w", addr, err)
	}
	return pb.NewHivemindAdminServiceClient(conn), conn, nil
}
