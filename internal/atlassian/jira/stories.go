package jira

import (
	"context"
)

// StoriesJQLSearch searches for stories in Jira using JQL.
//
// We request the `renderedFields` expand so Jira returns a pre-rendered HTML
// version of fields like `description`. That avoids having to interpret ADF
// (Atlassian Document Format) ourselves and produces image tags pointing at
// /secure/attachment/... which the frontend proxies for auth.
func (c *Client) StoriesJQLSearch(ctx context.Context, jql string, fields []string, startAt int, maxResults int) (*IssuesSearchResult, error) {
	iss := IssuesSearchResult{}

	issues, _, err := c.instance.Issue.Search.SearchJQL(ctx, jql, fields, []string{"renderedFields"}, maxResults, "")
	if err != nil {
		return nil, err
	}

	iss.Total = issues.Total
	iss.Issues = issues.Issues

	return &iss, err
}
