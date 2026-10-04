package opencode

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

type directoriesResponse struct {
	raw string
}

func getProjectDirectories(t *testing.T, base, path string) (int, directoriesResponse) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, base+path, nil)
	if err != nil {
		t.Fatalf("build GET %s: %v", path, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read GET %s: %v", path, err)
	}
	return resp.StatusCode, directoriesResponse{raw: string(data)}
}

func TestProjectDirectoriesServesDeclaredContract(t *testing.T) {
	_, srv, _, dir := newProjectCurrentTestServer(t)
	projectID := instanceID(dir)
	status, body := getProjectDirectories(t, srv.URL, "/project/"+projectID+"/directories")
	if status != http.StatusOK {
		t.Fatalf("GET project directories: got %d, want 200; body=%s", status, body.raw)
	}
	var directories []struct {
		Directory string `json:"directory"`
		Strategy  string `json:"strategy,omitempty"`
	}
	if err := json.Unmarshal([]byte(body.raw), &directories); err != nil {
		t.Fatalf("200 body is not ProjectDirectories: %v (%s)", err, body.raw)
	}
	if len(directories) != 1 || directories[0].Directory != dir {
		t.Fatalf("directories = %#v, want singleton workspace %q", directories, dir)
	}

	status, body = getProjectDirectories(t, srv.URL, "/project/"+projectID+"/directories?directory=%20%20%20")
	if status != http.StatusBadRequest {
		t.Fatalf("blank directory selector: got %d, want 400; body=%s", status, body.raw)
	}
	var bad struct {
		Name string `json:"name"`
		Data struct {
			Kind    string `json:"kind"`
			Message string `json:"message"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body.raw), &bad); err != nil {
		t.Fatalf("400 body is not BadRequestError: %v (%s)", err, body.raw)
	}
	if bad.Name != "BadRequest" || bad.Data.Kind != "Query" || bad.Data.Message == "" {
		t.Fatalf("400 body = %#v, want declared BadRequest Query envelope", bad)
	}
}
