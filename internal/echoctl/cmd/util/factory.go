package util

import (
	"net/http"
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
	HTTPClient() *http.Client
}

type defaultFactory struct {
}

func NewDefaultFactory() Factory {
	return &defaultFactory{}
}

func (f *defaultFactory) HTTPClient() *http.Client {
	return http.DefaultClient
}
