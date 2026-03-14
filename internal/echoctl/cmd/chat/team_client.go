package chat

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/kiosk404/echoryn/pkg/cli/tui/command"
	"github.com/kiosk404/echoryn/pkg/utils/json"
)

// TeamHTTPClient implements command.TeamAPI by calling the Hivemind /v1/teams HTTP API.
type TeamHTTPClient struct {
	baseURL    string
	sessionKey string
	httpClient *http.Client
}

// Ensure TeamHTTPClient implements command.TeamAPI at compile time.
var _ command.TeamAPI = (*TeamHTTPClient)(nil)

// NewTeamHTTPClient creates a new TeamHTTPClient.
func NewTeamHTTPClient(baseURL, sessionKey string, httpClient *http.Client) *TeamHTTPClient {
	return &TeamHTTPClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		sessionKey: sessionKey,
		httpClient: httpClient,
	}
}

// ListTemplates implements command.TeamAPI.
func (c *TeamHTTPClient) ListTemplates(ctx context.Context) ([]command.TemplateInfo, error) {
	var resp struct {
		Data []templateResponse `json:"data"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/v1/teams/templates", nil, &resp); err != nil {
		return nil, err
	}

	result := make([]command.TemplateInfo, 0, len(resp.Data))
	for _, t := range resp.Data {
		result = append(result, command.TemplateInfo{
			ID:          t.ID,
			Name:        t.Name,
			Description: t.Description,
			Strategy:    t.DefaultStrategy,
			MemberCount: len(t.Members),
		})
	}
	return result, nil
}

// CreateTeam implements command.TeamAPI.
func (c *TeamHTTPClient) CreateTeam(ctx context.Context, req command.CreateTeamReq) (*command.TeamState, error) {
	body := createTeamReq{
		TemplateID:      req.TemplateID,
		Name:            req.Name,
		TaskDescription: req.TaskDescription,
		Strategy:        req.Strategy,
	}

	var resp teamResponse
	if err := c.doJSON(ctx, http.MethodPost, "/v1/teams", body, &resp); err != nil {
		return nil, err
	}
	return toTeamState(&resp), nil
}

// GetTeam implements command.TeamAPI.
func (c *TeamHTTPClient) GetTeam(ctx context.Context, teamID string) (*command.TeamState, error) {
	var resp teamResponse
	if err := c.doJSON(ctx, http.MethodGet, "/v1/teams/"+teamID, nil, &resp); err != nil {
		return nil, err
	}
	return toTeamState(&resp), nil
}

// DissolveTeam implements command.TeamAPI.
func (c *TeamHTTPClient) DissolveTeam(ctx context.Context, teamID string) error {
	return c.doJSON(ctx, http.MethodDelete, "/v1/teams/"+teamID, nil, nil)
}

// SendMessage implements command.TeamAPI.
func (c *TeamHTTPClient) SendMessage(ctx context.Context, teamID, recipientLabel, content string) error {
	body := sendMessageReq{
		Recipient: recipientLabel,
		Content:   content,
	}
	return c.doJSON(ctx, http.MethodPost, "/v1/teams/"+teamID+"/messages", body, nil)
}

// Broadcast implements command.TeamAPI.
func (c *TeamHTTPClient) Broadcast(ctx context.Context, teamID, content string) error {
	body := sendMessageReq{
		Content:   content,
		Broadcast: true,
	}
	return c.doJSON(ctx, http.MethodPost, "/v1/teams/"+teamID+"/messages", body, nil)
}

// --- Internal HTTP helper ---

func (c *TeamHTTPClient) doJSON(ctx context.Context, method, path string, reqBody any, respBody any) error {
	var bodyReader io.Reader
	if reqBody != nil {
		data, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.sessionKey != "" {
		req.Header.Set("X-Session-Key", c.sessionKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		// Try to extract error message from response.
		var errResp struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(respData, &errResp) == nil && errResp.Message != "" {
			return fmt.Errorf("server error (%d): %s", resp.StatusCode, errResp.Message)
		}
		return fmt.Errorf("server error (%d): %s", resp.StatusCode, string(respData))
	}

	if respBody != nil && len(respData) > 0 {
		if err := json.Unmarshal(respData, respBody); err != nil {
			return fmt.Errorf("unmarshal response: %w", err)
		}
	}
	return nil
}

// --- Internal types (mirror server response) ---

type createTeamReq struct {
	TemplateID      string `json:"template_id,omitempty"`
	Name            string `json:"name,omitempty"`
	TaskDescription string `json:"task_description"`
	Strategy        string `json:"strategy,omitempty"`
}

type sendMessageReq struct {
	Recipient string `json:"recipient,omitempty"`
	Content   string `json:"content"`
	Broadcast bool   `json:"broadcast,omitempty"`
}

type templateResponse struct {
	ID              string                   `json:"id"`
	Name            string                   `json:"name"`
	Description     string                   `json:"description"`
	DefaultStrategy string                   `json:"default_strategy"`
	Members         []templateMemberResponse `json:"members"`
}

type templateMemberResponse struct {
	ID       string `json:"id"`
	Role     string `json:"role"`
	Label    string `json:"label"`
	IsLeader bool   `json:"is_leader"`
}

type teamResponse struct {
	ID       string           `json:"id"`
	Name     string           `json:"name"`
	Strategy string           `json:"strategy"`
	Status   string           `json:"status"`
	Members  []memberResponse `json:"members"`
}

type memberResponse struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	AgentID   string `json:"agent_id"`
	Label     string `json:"label"`
	Role      string `json:"role"`
	Status    string `json:"status"`
	IsLeader  bool   `json:"is_leader"`
	NodeID    string `json:"node_id"`
	Progress  string `json:"progress"`
}

func toTeamState(r *teamResponse) *command.TeamState {
	members := make([]command.TeamMemberState, 0, len(r.Members))
	for _, m := range r.Members {
		members = append(members, command.TeamMemberState{
			ID:        m.ID,
			SessionID: m.SessionID,
			Label:     m.Label,
			Role:      m.Role,
			Status:    m.Status,
			IsLeader:  m.IsLeader,
			NodeID:    m.NodeID,
			Progress:  m.Progress,
		})
	}
	return &command.TeamState{
		Enabled:    true,
		ID:         r.ID,
		Name:       r.Name,
		Strategy:   r.Strategy,
		Members:    members,
		FocusIndex: -1,
	}
}
