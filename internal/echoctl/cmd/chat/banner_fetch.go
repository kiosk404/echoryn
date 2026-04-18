package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/kiosk404/echoryn/pkg/cli/tui/render"
)

// fetchBannerData fetches tools, nodes, skills, and the default model name
// from the Hivemind server in parallel. Returns partial data on failure.
func fetchBannerData(ctx context.Context, baseURL string, httpClient *http.Client) (
	tools []render.ToolGroupInfo,
	nodes []render.GolemNodeInfo,
	skills []render.SkillGroupInfo,
	defaultModel string,
) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Second}
	}

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(4)

	go func() {
		defer wg.Done()
		tools = fetchToolGroups(ctx, baseURL, httpClient)
	}()
	go func() {
		defer wg.Done()
		nodes = fetchGolemNodes(ctx, baseURL, httpClient)
	}()
	go func() {
		defer wg.Done()
		skills = fetchSkillGroups(ctx, baseURL, httpClient)
	}()
	go func() {
		defer wg.Done()
		defaultModel = fetchDefaultModel(ctx, baseURL, httpClient)
	}()

	wg.Wait()
	return
}

func fetchToolGroups(ctx context.Context, baseURL string, client *http.Client) []render.ToolGroupInfo {
	type resp struct {
		Groups []struct {
			Category string   `json:"category"`
			Tools    []string `json:"tools"`
		} `json:"groups"`
	}
	var r resp
	if err := fetchJSON(ctx, baseURL+"/v1/tools", client, &r); err != nil {
		return nil
	}
	result := make([]render.ToolGroupInfo, 0, len(r.Groups))
	for _, g := range r.Groups {
		result = append(result, render.ToolGroupInfo{Category: g.Category, Tools: g.Tools})
	}
	return result
}

func fetchGolemNodes(ctx context.Context, baseURL string, client *http.Client) []render.GolemNodeInfo {
	type resp struct {
		Nodes []struct {
			ID     string            `json:"id"`
			Name   string            `json:"name"`
			Status string            `json:"status"`
			Labels map[string]string `json:"labels,omitempty"`
		} `json:"nodes"`
	}
	var r resp
	if err := fetchJSON(ctx, baseURL+"/v1/nodes", client, &r); err != nil {
		return nil
	}
	result := make([]render.GolemNodeInfo, 0, len(r.Nodes))
	for _, n := range r.Nodes {
		result = append(result, render.GolemNodeInfo{ID: n.ID, Name: n.Name, Status: n.Status, Labels: n.Labels})
	}
	return result
}

func fetchSkillGroups(ctx context.Context, baseURL string, client *http.Client) []render.SkillGroupInfo {
	type resp struct {
		Groups []struct {
			Source string   `json:"source"`
			Skills []string `json:"skills"`
		} `json:"groups"`
	}
	var r resp
	if err := fetchJSON(ctx, baseURL+"/v1/skills", client, &r); err != nil {
		return nil
	}
	result := make([]render.SkillGroupInfo, 0, len(r.Groups))
	for _, g := range r.Groups {
		result = append(result, render.SkillGroupInfo{Source: g.Source, Skills: g.Skills})
	}
	return result
}

// fetchDefaultModel gets the default model name from GET /v1/info.
func fetchDefaultModel(ctx context.Context, baseURL string, client *http.Client) string {
	type resp struct {
		DefaultModel string `json:"default_model"`
	}
	var r resp
	if err := fetchJSON(ctx, baseURL+"/v1/info", client, &r); err != nil {
		return ""
	}
	return r.DefaultModel
}

func fetchJSON(ctx context.Context, url string, client *http.Client, target interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

