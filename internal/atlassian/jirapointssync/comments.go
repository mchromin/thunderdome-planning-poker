package jirapointssync

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/StevenWeathers/thunderdome-planning-poker/thunderdome"
)

// IssueComment is a minimal projection of a Jira comment used by the UI.
type IssueComment struct {
	ID      string `json:"id"`
	Author  string `json:"author"`
	Created string `json:"created"`
	Updated string `json:"updated"`
	Body    string `json:"body"`
}

// jiraCommentsResponse models the subset of Jira's GET issue/{key}/comment response we need.
type jiraCommentsResponse struct {
	Comments []struct {
		ID     string `json:"id"`
		Body   string `json:"body"`
		Author struct {
			DisplayName string `json:"displayName"`
			Name        string `json:"name"`
		} `json:"author"`
		Created string `json:"created"`
		Updated string `json:"updated"`
	} `json:"comments"`
}

// FetchIssueComments returns the comments on a Jira issue, ordered as Jira returns them.
func FetchIssueComments(ctx context.Context, instance thunderdome.JiraInstance, issueKey string) ([]IssueComment, error) {
	if issueKey == "" {
		return nil, fmt.Errorf("jirapointssync: empty issue key")
	}
	host := strings.TrimRight(strings.TrimSpace(instance.Host), "/")
	if host == "" {
		return nil, fmt.Errorf("jirapointssync: jira instance has no host")
	}

	endpoint := fmt.Sprintf("%s/rest/api/2/issue/%s/comment", host, url.PathEscape(issueKey))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("jirapointssync: new request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if instance.JiraDataCenter {
		req.Header.Set("Authorization", "Bearer "+instance.AccessToken)
	} else {
		creds := instance.ClientMail + ":" + instance.AccessToken
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(creds)))
	}

	resp, err := activeClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("jirapointssync: do: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("jirapointssync: jira returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var out jiraCommentsResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("jirapointssync: decode: %w", err)
	}

	comments := make([]IssueComment, 0, len(out.Comments))
	for _, c := range out.Comments {
		author := c.Author.DisplayName
		if author == "" {
			author = c.Author.Name
		}
		comments = append(comments, IssueComment{
			ID:      c.ID,
			Author:  author,
			Created: c.Created,
			Updated: c.Updated,
			Body:    c.Body,
		})
	}
	return comments, nil
}
