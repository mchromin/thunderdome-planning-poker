package http

import (
	"context"

	"go.uber.org/zap"

	"github.com/StevenWeathers/thunderdome-planning-poker/internal/atlassian/jirapointssync"
	"github.com/StevenWeathers/thunderdome-planning-poker/thunderdome"
)

// pushPointsToJira best-effort updates the Jira story-points custom field for
// `story` using `userID`'s first configured Jira instance. Designed to be
// fail-open: every error is logged and swallowed so a Jira outage cannot
// prevent local poker finalization.
//
// `userID` is typically the facilitator who clicked Finalize, since their
// credentials are most likely permitted to edit the issue.
func (s *Service) pushPointsToJira(ctx context.Context, userID string, story *thunderdome.Story, points string) {
	if s == nil || s.JiraDataSvc == nil || story == nil {
		return
	}
	logger := s.Logger.Ctx(ctx)

	pts, ok := jirapointssync.ParsePoints(points)
	if !ok {
		// Symbolic or empty vote (e.g. "?", "☕️"); nothing to push.
		return
	}

	issueKey := jirapointssync.ResolveIssueKey(story.ReferenceID, story.Link)
	if issueKey == "" {
		return
	}

	instances, err := s.JiraDataSvc.FindInstancesByUserID(ctx, userID)
	if err != nil {
		logger.Warn("jira points sync: lookup instances failed",
			zap.Error(err), zap.String("user_id", userID), zap.String("issue", issueKey))
		return
	}
	if len(instances) == 0 {
		return
	}
	instance := instances[0]

	fieldKey := s.Config.JiraStoryPointsField
	if fieldKey == "" {
		fieldKey = "customfield_11204"
	}

	if err := jirapointssync.UpdateIssuePoints(ctx, instance, issueKey, pts, fieldKey); err != nil {
		logger.Warn("jira points sync: update failed",
			zap.Error(err),
			zap.String("issue", issueKey),
			zap.String("instance_id", instance.ID),
			zap.String("user_id", userID))
		return
	}
	logger.Info("jira points sync: updated",
		zap.String("issue", issueKey),
		zap.Float64("points", pts))
}
