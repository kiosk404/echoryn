package plugin

import (
	"fmt"
)

// Code is a status code returned by plugin operations.
// Modeled after K8s scheduler framework's Code type.
type Code int

const (
	// Success means the plugin operation completed successfully.
	Success Code = iota
	// Error means the plugin encountered a fatal error.
	Error
	// Skip means the plugin chose to skip this operation.
	Skip
	// Unschedulable means the operation cannot be completed (K8s terminology).
	Unschedulable
)

var codeNames = map[Code]string{
	Success:       "Success",
	Error:         "Error",
	Skip:          "Skip",
	Unschedulable: "Unschedulable",
}

// String returns the human-readable name of the code.
func (c Code) String() string {
	if name, ok := codeNames[c]; ok {
		return name
	}
	return fmt.Sprintf("Code(%d)", int(c))
}

// Status indicates the result of running a plugin operation.
// This is modeled after K8s scheduler framework's Status type.
type Status struct {
	code    Code
	reasons []string
	err     error
	plugin  string
}

// NewStatus creates a new Status with the given code and reasons.
func NewStatus(code Code, reasons ...string) *Status {
	return &Status{
		code:    code,
		reasons: reasons,
	}
}

// NewStatusWithError creates an error Status from an error.
func NewStatusWithError(err error) *Status {
	return &Status{
		code:    Error,
		reasons: []string{err.Error()},
		err:     err,
	}
}

// Code returns the status code.
func (s *Status) Code() Code {
	if s == nil {
		return Success
	}
	return s.code
}

// IsSuccess returns true if the status code is Success.
func (s *Status) IsSuccess() bool {
	return s.Code() == Success
}

// Message returns a concatenated message from all reasons.
func (s *Status) Message() string {
	if s == nil {
		return ""
	}
	if len(s.reasons) == 0 {
		return s.code.String()
	}
	msg := ""
	for i, reason := range s.reasons {
		if i > 0 {
			msg += ", "
		}
		msg += reason
	}
	return msg
}

// Err returns the underlying error, if any.
func (s *Status) Err() error {
	if s == nil {
		return nil
	}
	if s.err != nil {
		return s.err
	}
	if s.code == Success || s.code == Skip {
		return nil
	}
	return fmt.Errorf("plugin %q returned status %s: %s", s.plugin, s.code, s.Message())
}

// WithPlugin sets the plugin name on the status (for diagnostics).
func (s *Status) WithPlugin(name string) *Status {
	if s == nil {
		return nil
	}
	s.plugin = name
	return s
}

// AsError converts any non-success status to an error.
func (s *Status) AsError() error {
	return s.Err()
}
