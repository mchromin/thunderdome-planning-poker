package http

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"go.uber.org/zap"

	"github.com/StevenWeathers/thunderdome-planning-poker/internal/atlassian/jirapointssync"
	"github.com/StevenWeathers/thunderdome-planning-poker/thunderdome"
)

// handleJiraIssueComments returns the comments for a Jira issue using the
// requesting user's stored Jira credentials.
//
// Query parameters:
//   - host (required): the Jira host. Must match one of the user's instances.
//   - key  (required): the issue key, e.g. "MFD-6571".
//
//	@Summary		Fetch Jira issue comments using the user's Jira credentials
//	@Tags			jira
//	@Param			userId	path	string	true	"the user id"
//	@Param			host	query	string	true	"Jira host, e.g. https://jira.example.com"
//	@Param			key		query	string	true	"Jira issue key"
//	@Success		200		object	standardJsonResponse{data=[]jirapointssync.IssueComment}
//	@Failure		400		object	standardJsonResponse{}
//	@Failure		502		object	standardJsonResponse{}
//	@Security		ApiKeyAuth
//	@Router			/users/{userId}/jira-comments [get]
func (s *Service) handleJiraIssueComments() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		sessionUserID, _ := ctx.Value(contextKeyUserID).(string)

		hostRaw := strings.TrimSpace(r.URL.Query().Get("host"))
		key := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("key")))
		if hostRaw == "" || key == "" {
			s.Failure(w, r, http.StatusBadRequest, Errorf(EINVALID, "host and key are required"))
			return
		}

		hostURL, err := url.Parse(strings.TrimRight(hostRaw, "/"))
		if err != nil || hostURL.Host == "" {
			s.Failure(w, r, http.StatusBadRequest, Errorf(EINVALID, "invalid host"))
			return
		}

		instances, err := s.JiraDataSvc.FindInstancesByUserID(ctx, sessionUserID)
		if err != nil {
			s.Logger.Ctx(ctx).Warn("jira comments: lookup failed", zap.Error(err))
			s.Failure(w, r, http.StatusBadGateway, Errorf(EINTERNAL, "lookup failed"))
			return
		}

		var matched *thunderdome.JiraInstance
		for i := range instances {
			ih, err := url.Parse(strings.TrimRight(instances[i].Host, "/"))
			if err != nil {
				continue
			}
			if strings.EqualFold(ih.Host, hostURL.Host) {
				matched = &instances[i]
				break
			}
		}
		if matched == nil {
			s.Failure(w, r, http.StatusBadRequest, Errorf(EINVALID, "host does not match a known Jira instance"))
			return
		}

		comments, err := jirapointssync.FetchIssueComments(ctx, *matched, key)
		if err != nil {
			s.Logger.Ctx(ctx).Warn("jira comments: fetch failed",
				zap.Error(err), zap.String("issue", key))
			s.Failure(w, r, http.StatusBadGateway, Errorf(EINTERNAL, fmt.Sprintf("upstream fetch failed: %v", err)))
			return
		}

		s.Success(w, r, http.StatusOK, comments, nil)
	}
}
