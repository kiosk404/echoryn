package util

import (
	"context"
)

// Factory provides abstractions that allow the echoctl command to be extended across multiple types
// of resources and different API sets.
// The rings are here for a reason. In order for composers to be able to provide alternative factory implementations
// they need to provide low level pieces of *certain* functions so that when the factory calls back into itself
// it uses the custom version of the function. Rather than try to enumerate everything that someone would want to
// override
// we split the factory into rings, where each ring can depend on methods in an earlier ring, but cannot depend
// upon peer methods in its own ring.
// commands are decoupled from the factory).
type Factory interface {
	HivemindConnector() HivemindConnector
	NodeChecker() NodeChecker
	NodeInfoCollector() NodeInfoCollector
}

type HivemindConnector interface {
	Connect(ctx context.Context, addr string) error
	Join(ctx context.Context, token string)
	NodeInfoCollector() NodeInfoCollector
}

type NodeInfoCollector interface {
	Collect(ctx context.Context) error
}

type NodeChecker interface {
	RunAll(ctx context.Context) ([]*NodeCheckResult, error)

	RunChecker(ctx context.Context, name string) (*NodeCheckResult, error)
}

type NodeCheckResult struct {
	Name    string
	Status  string
	Message string
	Passed  bool
	Errors  []error
}

type defaultFactory struct {
}

func NewDefaultFactory() Factory {
	return &defaultFactory{}
}

func (d *defaultFactory) HivemindConnector() HivemindConnector {
	return &defaultHivemindConnector{}
}

func (d *defaultFactory) NodeInfoCollector() NodeInfoCollector {
	return &defaultNodeInfoCollector{}
}

func (d *defaultFactory) NodeChecker() NodeChecker {
	return &defaultNodeChecker{}
}

type defaultHivemindConnector struct {
}

func (d defaultHivemindConnector) Connect(ctx context.Context, addr string) error {
	//TODO implement me
	panic("implement me")
}

func (d defaultHivemindConnector) Join(ctx context.Context, token string) {
	//TODO implement me
	panic("implement me")
}

func (d defaultHivemindConnector) NodeInfoCollector() NodeInfoCollector {
	//TODO implement me
	panic("implement me")
}

type defaultNodeInfoCollector struct {
}

func (d defaultNodeInfoCollector) Collect(ctx context.Context) error {
	//TODO implement me
	panic("implement me")
}

type defaultNodeChecker struct {
}

func (d defaultNodeChecker) RunAll(ctx context.Context) ([]*NodeCheckResult, error) {
	//TODO implement me
	panic("implement me")
}

func (d defaultNodeChecker) RunChecker(ctx context.Context, name string) (*NodeCheckResult, error) {
	//TODO implement me
	panic("implement me")
}
