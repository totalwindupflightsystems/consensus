package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is a typed REST client for the Consensus API.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// NewClient creates a new API client with a 30-second request timeout.
func NewClient(serverURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(serverURL, "/"),
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) do(method, path string, body any) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}

	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.http.Do(req)
}

func (c *Client) get(path string) (*http.Response, error) {
	return c.do(http.MethodGet, path, nil)
}

func (c *Client) post(path string, body any) (*http.Response, error) {
	return c.do(http.MethodPost, path, body)
}

func (c *Client) patch(path string, body any) (*http.Response, error) {
	return c.do(http.MethodPatch, path, body)
}

func (c *Client) delete(path string) (*http.Response, error) {
	return c.do(http.MethodDelete, path, nil)
}

func (c *Client) decodeBody(resp *http.Response, target any) error {
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode >= http.StatusBadRequest {
		var errResp struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(data, &errResp) == nil && errResp.Error.Message != "" {
			return fmt.Errorf("%s: %s", errResp.Error.Code, errResp.Error.Message)
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(data))
	}

	if target != nil {
		return json.Unmarshal(data, target)
	}
	return nil
}

// CreateSession creates a new Consensus session.
func (c *Client) CreateSession(req CreateSessionRequest) (*CreateSessionResponse, error) {
	resp, err := c.post("/api/v1/sessions", req)
	if err != nil {
		return nil, err
	}
	var result CreateSessionResponse
	return &result, c.decodeBody(resp, &result)
}

// ListSessions lists all Consensus sessions.
func (c *Client) ListSessions() ([]SessionResponse, error) {
	resp, err := c.get("/api/v1/sessions")
	if err != nil {
		return nil, err
	}
	var result []SessionResponse
	return result, c.decodeBody(resp, &result)
}

// GetSession retrieves a session by ID.
func (c *Client) GetSession(sessionID string) (*SessionResponse, error) {
	resp, err := c.get("/api/v1/sessions/" + sessionID)
	if err != nil {
		return nil, err
	}
	var result SessionResponse
	return &result, c.decodeBody(resp, &result)
}

// UpdateSession updates a session status.
func (c *Client) UpdateSession(sessionID string, req UpdateSessionRequest) (*SessionResponse, error) {
	resp, err := c.patch("/api/v1/sessions/"+sessionID, req)
	if err != nil {
		return nil, err
	}
	var result SessionResponse
	return &result, c.decodeBody(resp, &result)
}

// DeleteSession deletes a session by ID.
func (c *Client) DeleteSession(sessionID string) error {
	resp, err := c.delete("/api/v1/sessions/" + sessionID)
	if err != nil {
		return err
	}
	return c.decodeBody(resp, nil)
}

// SendMessage sends a message to a session.
func (c *Client) SendMessage(sessionID string, req SendMessageRequest) (*SessionResponse, error) {
	resp, err := c.post("/api/v1/sessions/"+sessionID+"/message", req)
	if err != nil {
		return nil, err
	}
	var result SessionResponse
	return &result, c.decodeBody(resp, &result)
}

// ListMemory lists memory events for a session.
func (c *Client) ListMemory(sessionID string) ([]MemoryEventResponse, error) {
	resp, err := c.get("/api/v1/sessions/" + sessionID + "/memory")
	if err != nil {
		return nil, err
	}
	var result []MemoryEventResponse
	return result, c.decodeBody(resp, &result)
}

// GetActiveContext retrieves the active context for a session.
func (c *Client) GetActiveContext(sessionID string) ([]ActiveContextResponse, error) {
	resp, err := c.get("/api/v1/sessions/" + sessionID + "/context")
	if err != nil {
		return nil, err
	}
	var result []ActiveContextResponse
	return result, c.decodeBody(resp, &result)
}

// ListIterations lists iteration commits for a session.
func (c *Client) ListIterations(sessionID string) ([]IterationCommitResponse, error) {
	resp, err := c.get("/api/v1/sessions/" + sessionID + "/iterations")
	if err != nil {
		return nil, err
	}
	var result []IterationCommitResponse
	return result, c.decodeBody(resp, &result)
}

// GetMemoryEvent retrieves one memory event from a session.
func (c *Client) GetMemoryEvent(sessionID, memoryID string) (*MemoryEventResponse, error) {
	resp, err := c.get("/api/v1/sessions/" + sessionID + "/memory/" + memoryID)
	if err != nil {
		return nil, err
	}
	var result MemoryEventResponse
	return &result, c.decodeBody(resp, &result)
}

// ListTasks lists tasks for a session.
func (c *Client) ListTasks(sessionID string) ([]TaskResponse, error) {
	resp, err := c.get("/api/v1/sessions/" + sessionID + "/tasks")
	if err != nil {
		return nil, err
	}
	var result []TaskResponse
	return result, c.decodeBody(resp, &result)
}

// CreateTask creates a task for a session.
func (c *Client) CreateTask(sessionID string, req CreateTaskRequest) (*TaskResponse, error) {
	resp, err := c.post("/api/v1/sessions/"+sessionID+"/tasks", req)
	if err != nil {
		return nil, err
	}
	var result TaskResponse
	return &result, c.decodeBody(resp, &result)
}

// UpdateTask updates a task status.
func (c *Client) UpdateTask(taskID string, req UpdateTaskRequest) (*TaskResponse, error) {
	resp, err := c.patch("/api/v1/tasks/"+taskID, req)
	if err != nil {
		return nil, err
	}
	var result TaskResponse
	return &result, c.decodeBody(resp, &result)
}

// ListTools lists available tools.
func (c *Client) ListTools() ([]ToolResponse, error) {
	resp, err := c.get("/api/v1/tools")
	if err != nil {
		return nil, err
	}
	var result []ToolResponse
	return result, c.decodeBody(resp, &result)
}

// ListSkills lists available skills.
func (c *Client) ListSkills() ([]SkillResponse, error) {
	resp, err := c.get("/api/v1/skills")
	if err != nil {
		return nil, err
	}
	var result []SkillResponse
	return result, c.decodeBody(resp, &result)
}

// GetSkill retrieves a skill by name.
func (c *Client) GetSkill(skillName string) (*SkillDetailResponse, error) {
	resp, err := c.get("/api/v1/skills/" + skillName)
	if err != nil {
		return nil, err
	}
	var result SkillDetailResponse
	return &result, c.decodeBody(resp, &result)
}

// ListApprovals lists all approval requests.
func (c *Client) ListApprovals() ([]ApprovalResponse, error) {
	resp, err := c.get("/api/v1/approvals")
	if err != nil {
		return nil, err
	}
	var result []ApprovalResponse
	return result, c.decodeBody(resp, &result)
}

// GetApproval retrieves an approval request by ID.
func (c *Client) GetApproval(approvalID string) (*ApprovalResponse, error) {
	resp, err := c.get("/api/v1/approvals/" + approvalID)
	if err != nil {
		return nil, err
	}
	var result ApprovalResponse
	return &result, c.decodeBody(resp, &result)
}

// ReviewApproval reviews an approval request.
func (c *Client) ReviewApproval(approvalID string, req ApprovalReviewRequest) (*ApprovalResponse, error) {
	resp, err := c.post("/api/v1/approvals/"+approvalID+"/review", req)
	if err != nil {
		return nil, err
	}
	var result ApprovalResponse
	return &result, c.decodeBody(resp, &result)
}

// SessionApprovals lists approval requests for a session.
func (c *Client) SessionApprovals(sessionID string) ([]ApprovalResponse, error) {
	resp, err := c.get("/api/v1/sessions/" + sessionID + "/approvals")
	if err != nil {
		return nil, err
	}
	var result []ApprovalResponse
	return result, c.decodeBody(resp, &result)
}

// GetSessionBilling retrieves dynamic billing data for a session.
func (c *Client) GetSessionBilling(sessionID string) (map[string]any, error) {
	resp, err := c.get("/api/v1/sessions/" + sessionID + "/billing")
	if err != nil {
		return nil, err
	}
	var result map[string]any
	return result, c.decodeBody(resp, &result)
}

// GetConfig retrieves the current API configuration.
func (c *Client) GetConfig() (*ConfigResponse, error) {
	resp, err := c.get("/api/v1/config")
	if err != nil {
		return nil, err
	}
	var result ConfigResponse
	return &result, c.decodeBody(resp, &result)
}

// GetMetrics retrieves API metrics.
func (c *Client) GetMetrics() (*MetricsResponse, error) {
	resp, err := c.get("/api/v1/metrics")
	if err != nil {
		return nil, err
	}
	var result MetricsResponse
	return &result, c.decodeBody(resp, &result)
}

// Health retrieves the API health status.
func (c *Client) Health() (*HealthResponse, error) {
	resp, err := c.get("/api/v1/health")
	if err != nil {
		return nil, err
	}
	var result HealthResponse
	return &result, c.decodeBody(resp, &result)
}
