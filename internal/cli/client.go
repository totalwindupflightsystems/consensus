// CLI API client — thin REST client for CLI commands.
//
// axiom:trace work_item=interfaces-api-cli-01 spec=specs/016-cli-interface.md plan=phase-4/task-4-1/step-4-1-2 impl=internal/cli/client.go
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is a thin REST client for the Consensus API (SPEC-015).
// CLI commands use this to interact with the running server.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// NewClient creates a new API client.
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

func (c *Client) get(path string) (*http.Response, error)  { return c.do("GET", path, nil) }
func (c *Client) post(path string, body any) (*http.Response, error) { return c.do("POST", path, body) }
func (c *Client) patch(path string, body any) (*http.Response, error) { return c.do("PATCH", path, body) }
func (c *Client) delete(path string) (*http.Response, error) { return c.do("DELETE", path, nil) }

func (c *Client) decodeBody(resp *http.Response, target any) error {
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode >= 400 {
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

// ─── Session Operations ─────────────────────────────────────────────

func (c *Client) CreateSession(req map[string]any) (map[string]any, error) {
	resp, err := c.post("/api/v1/sessions", req)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	return result, c.decodeBody(resp, &result)
}

func (c *Client) ListSessions() ([]map[string]any, error) {
	resp, err := c.get("/api/v1/sessions")
	if err != nil {
		return nil, err
	}
	var result []map[string]any
	return result, c.decodeBody(resp, &result)
}

func (c *Client) GetSession(sessionID string) (map[string]any, error) {
	resp, err := c.get("/api/v1/sessions/" + sessionID)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	return result, c.decodeBody(resp, &result)
}

func (c *Client) UpdateSession(sessionID string, req map[string]any) (map[string]any, error) {
	resp, err := c.patch("/api/v1/sessions/"+sessionID, req)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	return result, c.decodeBody(resp, &result)
}

func (c *Client) DeleteSession(sessionID string) error {
	resp, err := c.delete("/api/v1/sessions/" + sessionID)
	if err != nil {
		return err
	}
	return c.decodeBody(resp, nil)
}

func (c *Client) SendMessage(sessionID string, req map[string]any) (map[string]any, error) {
	resp, err := c.post("/api/v1/sessions/"+sessionID+"/message", req)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	return result, c.decodeBody(resp, &result)
}

// ─── Memory & Context ────────────────────────────────────────────────

func (c *Client) ListMemory(sessionID string) ([]map[string]any, error) {
	resp, err := c.get("/api/v1/sessions/" + sessionID + "/memory")
	if err != nil {
		return nil, err
	}
	var result []map[string]any
	return result, c.decodeBody(resp, &result)
}

func (c *Client) GetActiveContext(sessionID string) ([]map[string]any, error) {
	resp, err := c.get("/api/v1/sessions/" + sessionID + "/context")
	if err != nil {
		return nil, err
	}
	var result []map[string]any
	return result, c.decodeBody(resp, &result)
}

func (c *Client) ListIterations(sessionID string) ([]map[string]any, error) {
	resp, err := c.get("/api/v1/sessions/" + sessionID + "/iterations")
	if err != nil {
		return nil, err
	}
	var result []map[string]any
	return result, c.decodeBody(resp, &result)
}

func (c *Client) GetMemoryEvent(sessionID, memoryID string) (map[string]any, error) {
	resp, err := c.get("/api/v1/sessions/" + sessionID + "/memory/" + memoryID)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	return result, c.decodeBody(resp, &result)
}

// ─── Tasks ───────────────────────────────────────────────────────────

func (c *Client) ListTasks(sessionID string) ([]map[string]any, error) {
	resp, err := c.get("/api/v1/sessions/" + sessionID + "/tasks")
	if err != nil {
		return nil, err
	}
	var result []map[string]any
	return result, c.decodeBody(resp, &result)
}

func (c *Client) CreateTask(sessionID string, req map[string]any) (map[string]any, error) {
	resp, err := c.post("/api/v1/sessions/"+sessionID+"/tasks", req)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	return result, c.decodeBody(resp, &result)
}

func (c *Client) UpdateTask(taskID string, req map[string]any) (map[string]any, error) {
	resp, err := c.patch("/api/v1/tasks/"+taskID, req)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	return result, c.decodeBody(resp, &result)
}

// ─── Tools & Skills ──────────────────────────────────────────────────

func (c *Client) ListTools() ([]map[string]any, error) {
	resp, err := c.get("/api/v1/tools")
	if err != nil {
		return nil, err
	}
	var result []map[string]any
	return result, c.decodeBody(resp, &result)
}

func (c *Client) ListSkills() ([]map[string]any, error) {
	resp, err := c.get("/api/v1/skills")
	if err != nil {
		return nil, err
	}
	var result []map[string]any
	return result, c.decodeBody(resp, &result)
}

func (c *Client) GetSkill(skillName string) (map[string]any, error) {
	resp, err := c.get("/api/v1/skills/" + skillName)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	return result, c.decodeBody(resp, &result)
}

// ─── Approvals ───────────────────────────────────────────────────────

func (c *Client) ListApprovals() ([]map[string]any, error) {
	resp, err := c.get("/api/v1/approvals")
	if err != nil {
		return nil, err
	}
	var result []map[string]any
	return result, c.decodeBody(resp, &result)
}

func (c *Client) GetApproval(approvalID string) (map[string]any, error) {
	resp, err := c.get("/api/v1/approvals/" + approvalID)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	return result, c.decodeBody(resp, &result)
}

func (c *Client) ReviewApproval(approvalID string, req map[string]any) (map[string]any, error) {
	resp, err := c.post("/api/v1/approvals/"+approvalID+"/review", req)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	return result, c.decodeBody(resp, &result)
}

func (c *Client) SessionApprovals(sessionID string) ([]map[string]any, error) {
	resp, err := c.get("/api/v1/sessions/" + sessionID + "/approvals")
	if err != nil {
		return nil, err
	}
	var result []map[string]any
	return result, c.decodeBody(resp, &result)
}

// ─── Billing ────────────────────────────────────────────────────────

func (c *Client) GetSessionBilling(sessionID string) (map[string]any, error) {
	resp, err := c.get("/api/v1/sessions/" + sessionID + "/billing")
	if err != nil {
		return nil, err
	}
	var result map[string]any
	return result, c.decodeBody(resp, &result)
}

// ─── Config & Metrics ────────────────────────────────────────────────

func (c *Client) GetConfig() (map[string]any, error) {
	resp, err := c.get("/api/v1/config")
	if err != nil {
		return nil, err
	}
	var result map[string]any
	return result, c.decodeBody(resp, &result)
}

func (c *Client) GetMetrics() (map[string]any, error) {
	resp, err := c.get("/api/v1/metrics")
	if err != nil {
		return nil, err
	}
	var result map[string]any
	return result, c.decodeBody(resp, &result)
}

// ─── Health ──────────────────────────────────────────────────────────

func (c *Client) Health() (map[string]any, error) {
	resp, err := c.get("/api/v1/health")
	if err != nil {
		return nil, err
	}
	var result map[string]any
	return result, c.decodeBody(resp, &result)
}
