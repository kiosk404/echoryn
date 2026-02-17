package v1

import (
	"net/http"

	"github.com/kiosk404/echoryn/pkg/errorx"
)

// Hivemind handler error codes.
// Code format: 1XXYYZ
//   - 1:  module prefix (hivemind handler)
//   - XX: resource group (00=common, 01=chat, 02=agent, 03=session, 04=model)
//   - YY: sequential error number
//   - Z:  reserved (0)

const (
	// Common request errors (100xxx).
	ErrBind       = 100001
	ErrValidation = 100002

	// Chat completions errors (1001xx).
	ErrMessagesEmpty   = 100101
	ErrNoUserMessage   = 100102
	ErrEnsureAgent     = 100103
	ErrAgentRun        = 100104
	ErrStreamRecv      = 100105
	ErrNonStreamResult = 100106

	// Agent errors (1002xx).
	ErrAgentNotFound = 100201
	ErrAgentCreate   = 100202
	ErrAgentList     = 100203
	ErrAgentDelete   = 100204

	// Session errors (1003xx).
	ErrSessionNotFound = 100301
	ErrSessionList     = 100302
	ErrSessionDelete   = 100303

	// Model errors (1004xx).
	ErrModelList = 100401
)

func init() {
	// Common.
	errorx.MustRegister(newCoder(ErrBind, http.StatusBadRequest, "Request body binding failed"))
	errorx.MustRegister(newCoder(ErrValidation, http.StatusBadRequest, "Request validation failed"))

	// Chat completions.
	errorx.MustRegister(newCoder(ErrMessagesEmpty, http.StatusBadRequest, "Messages array is required and must not be empty"))
	errorx.MustRegister(newCoder(ErrNoUserMessage, http.StatusBadRequest, "No user message found in messages array"))
	errorx.MustRegister(newCoder(ErrEnsureAgent, http.StatusInternalServerError, "Failed to ensure agent"))
	errorx.MustRegister(newCoder(ErrAgentRun, http.StatusInternalServerError, "Agent run failed"))
	errorx.MustRegister(newCoder(ErrStreamRecv, http.StatusInternalServerError, "Stream receive error"))
	errorx.MustRegister(newCoder(ErrNonStreamResult, http.StatusInternalServerError, "Non-stream result error"))

	// Agent.
	errorx.MustRegister(newCoder(ErrAgentNotFound, http.StatusNotFound, "Agent not found"))
	errorx.MustRegister(newCoder(ErrAgentCreate, http.StatusInternalServerError, "Failed to create agent"))
	errorx.MustRegister(newCoder(ErrAgentList, http.StatusInternalServerError, "Failed to list agents"))
	errorx.MustRegister(newCoder(ErrAgentDelete, http.StatusInternalServerError, "Failed to delete agent"))

	// Session.
	errorx.MustRegister(newCoder(ErrSessionNotFound, http.StatusNotFound, "Session not found"))
	errorx.MustRegister(newCoder(ErrSessionList, http.StatusInternalServerError, "Failed to list sessions"))
	errorx.MustRegister(newCoder(ErrSessionDelete, http.StatusInternalServerError, "Failed to delete session"))

	// Model.
	errorx.MustRegister(newCoder(ErrModelList, http.StatusInternalServerError, "Failed to list models"))
}

type coder struct {
	code int
	http int
	msg  string
}

func newCoder(code, httpStatus int, msg string) *coder {
	return &coder{code: code, http: httpStatus, msg: msg}
}

func (c *coder) Code() int         { return c.code }
func (c *coder) HTTPStatus() int   { return c.http }
func (c *coder) String() string    { return c.msg }
func (c *coder) Reference() string { return "" }
