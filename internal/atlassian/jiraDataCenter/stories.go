package jiradatacenter

import (
	"context"

	jira_data_center "github.com/andygrunwald/go-jira/v2/onpremise"
)

// StoriesJQLSearch searches for stories in Jira using JQL.
//
// We pass `expand=renderedFields` so Jira returns a pre-rendered HTML version
// of fields like `description` (in `renderedFields.description`). That HTML
// is far easier to display correctly than reverse-engineering Jira wiki
// markup ourselves and includes proper <img> tags pointing at
// /secure/attachment/... which the frontend later proxies for auth.
func (c *Client) StoriesJQLSearch(ctx context.Context, jql string, fields []string, startAt int, maxResults int) (*IssuesSearchResult, error) {
	iss := IssuesSearchResult{}
	opt := &jira_data_center.SearchOptions{
		MaxResults: maxResults, // Max results can go up to 1000
		StartAt:    startAt,
		Expand:     "renderedFields",
		Fields:     fields,
	}
	issues, respo, err := c.instance.Issue.Search(ctx, jql, opt)

	if err != nil {
		return nil, err
	}

	iss.Total = respo.Total
	iss.Issues = issues

	return &iss, err
}
