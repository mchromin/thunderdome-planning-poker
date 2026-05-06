package http

import (
	"encoding/json"
	"io"
	"net/http"

	"go.uber.org/zap"

	"github.com/StevenWeathers/thunderdome-planning-poker/thunderdome"
)

// isFacilitatorByID checks the loaded poker game's facilitators slice.
func isFacilitatorByID(game *thunderdome.Poker, userID string) bool {
	for _, f := range game.Facilitators {
		if f == userID {
			return true
		}
	}
	return false
}

// asyncGameVisible used to filter votes/comments per user before responding;
// that responsibility now lives in the data layer (GetGameByID) so the same
// visibility rules apply to WebSocket init as well as REST. We keep this
// function as a small helper that hides the facilitator code from
// non-facilitators in the response.
func (s *Service) asyncGameVisible(game *thunderdome.Poker, userID string) *thunderdome.Poker {
	if game == nil {
		return nil
	}
	if !isFacilitatorByID(game, userID) {
		game.FacilitatorCode = ""
	}
	return game
}

// handleAsyncGetGame returns full async game state with visibility filtering.
//
//	@Summary		Get Async Poker Game
//	@Description	Get full state of an async poker game with per-user visibility applied
//	@Tags			poker,async
//	@Produce		json
//	@Param			battleId	path	string	true	"the poker game ID"
//	@Success		200			object	standardJsonResponse{data=thunderdome.Poker}
//	@Failure		403			object	standardJsonResponse{}
//	@Failure		404			object	standardJsonResponse{}
//	@Security		ApiKeyAuth
//	@Router			/battles/{battleId}/async [get]
func (s *Service) handleAsyncGetGame() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		sessionUserID := ctx.Value(contextKeyUserID).(string)
		userType := ctx.Value(contextKeyUserType).(string)

		gameID := r.PathValue("battleId")
		if idErr := validate.Var(gameID, "required,uuid"); idErr != nil {
			s.Failure(w, r, http.StatusBadRequest, Errorf(EINVALID, idErr.Error()))
			return
		}

		game, err := s.PokerDataSvc.GetGameByID(gameID, sessionUserID)
		if err != nil {
			s.Failure(w, r, http.StatusNotFound, Errorf(ENOTFOUND, "BATTLE_NOT_FOUND"))
			return
		}

		if game.JoinCode != "" {
			userErr := s.PokerDataSvc.GetUserActiveStatus(gameID, sessionUserID)
			if userErr != nil && userErr.Error() != "DUPLICATE_BATTLE_USER" && userType != thunderdome.AdminUserType {
				s.Failure(w, r, http.StatusForbidden, Errorf(EUNAUTHORIZED, "USER_MUST_JOIN_BATTLE"))
				return
			}
		}

		if game.SessionMode != thunderdome.PokerSessionModeAsync {
			s.Failure(w, r, http.StatusBadRequest, Errorf(EINVALID, "NOT_ASYNC_SESSION"))
			return
		}

		// ensure user is registered in the game so they can participate
		if _, addErr := s.PokerDataSvc.AddUser(gameID, sessionUserID); addErr != nil {
			s.Logger.Ctx(ctx).Warn("async add user error", zap.Error(addErr),
				zap.String("poker_id", gameID), zap.String("user_id", sessionUserID))
		}

		s.Success(w, r, http.StatusOK, s.asyncGameVisible(game, sessionUserID), nil)
	}
}

type asyncVoteRequestBody struct {
	Value string `json:"value" validate:"required"`
}

func (s *Service) confirmAsyncParticipant(w http.ResponseWriter, r *http.Request, gameID, userID string) (*thunderdome.Poker, bool) {
	game, err := s.PokerDataSvc.GetGameByID(gameID, userID)
	if err != nil {
		s.Failure(w, r, http.StatusNotFound, Errorf(ENOTFOUND, "BATTLE_NOT_FOUND"))
		return nil, false
	}
	if game.SessionMode != thunderdome.PokerSessionModeAsync {
		s.Failure(w, r, http.StatusBadRequest, Errorf(EINVALID, "NOT_ASYNC_SESSION"))
		return nil, false
	}
	if game.JoinCode != "" {
		userErr := s.PokerDataSvc.GetUserActiveStatus(gameID, userID)
		userType := r.Context().Value(contextKeyUserType).(string)
		if userErr != nil && userErr.Error() != "DUPLICATE_BATTLE_USER" && userType != thunderdome.AdminUserType {
			s.Failure(w, r, http.StatusForbidden, Errorf(EUNAUTHORIZED, "USER_MUST_JOIN_BATTLE"))
			return nil, false
		}
	}
	return game, true
}

func findStory(game *thunderdome.Poker, storyID string) *thunderdome.Story {
	for _, st := range game.Stories {
		if st.ID == storyID {
			return st
		}
	}
	return nil
}

// handleAsyncSetVote handles voting on an async story.
//
//	@Summary		Cast Async Vote
//	@Description	Set the requesting user's vote on a non-finalized async story
//	@Tags			poker,async
//	@Param			battleId	path	string					true	"the poker game ID"
//	@Param			planId		path	string					true	"the story ID"
//	@Param			body		body	asyncVoteRequestBody	true	"the vote payload"
//	@Success		200			object	standardJsonResponse{data=[]thunderdome.Story}
//	@Security		ApiKeyAuth
//	@Router			/battles/{battleId}/stories/{storyId}/vote [post]
func (s *Service) handleAsyncSetVote() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		sessionUserID := ctx.Value(contextKeyUserID).(string)
		gameID := r.PathValue("battleId")
		storyID := r.PathValue("storyId")
		if idErr := validate.Var(gameID, "required,uuid"); idErr != nil {
			s.Failure(w, r, http.StatusBadRequest, Errorf(EINVALID, idErr.Error()))
			return
		}
		if idErr := validate.Var(storyID, "required,uuid"); idErr != nil {
			s.Failure(w, r, http.StatusBadRequest, Errorf(EINVALID, idErr.Error()))
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			s.Failure(w, r, http.StatusBadRequest, Errorf(EINVALID, err.Error()))
			return
		}
		var rb asyncVoteRequestBody
		if err := json.Unmarshal(body, &rb); err != nil {
			s.Failure(w, r, http.StatusBadRequest, Errorf(EINVALID, err.Error()))
			return
		}
		if err := validate.Struct(rb); err != nil {
			s.Failure(w, r, http.StatusBadRequest, Errorf(EINVALID, err.Error()))
			return
		}

		game, ok := s.confirmAsyncParticipant(w, r, gameID, sessionUserID)
		if !ok {
			return
		}
		st := findStory(game, storyID)
		if st == nil {
			s.Failure(w, r, http.StatusNotFound, Errorf(ENOTFOUND, "STORY_NOT_FOUND"))
			return
		}
		if st.Points != "" || st.Skipped {
			s.Failure(w, r, http.StatusBadRequest, Errorf(EINVALID, "STORY_FINALIZED"))
			return
		}

		// ensure participant is registered
		if _, addErr := s.PokerDataSvc.AddUser(gameID, sessionUserID); addErr != nil {
			s.Logger.Ctx(ctx).Warn("async add user error", zap.Error(addErr))
		}

		stories, _ := s.PokerDataSvc.SetVote(gameID, sessionUserID, storyID, rb.Value)

		// reload game so visibility is applied with comments
		updated, err := s.PokerDataSvc.GetGameByID(gameID, sessionUserID)
		if err == nil && updated != nil {
			s.Success(w, r, http.StatusOK, s.asyncGameVisible(updated, sessionUserID), nil)
			return
		}
		s.Success(w, r, http.StatusOK, stories, nil)
	}
}

// handleAsyncRetractVote retracts the requesting user's vote on an async story.
//
//	@Summary		Retract Async Vote
//	@Tags			poker,async
//	@Security		ApiKeyAuth
//	@Router			/battles/{battleId}/stories/{storyId}/vote [delete]
func (s *Service) handleAsyncRetractVote() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		sessionUserID := ctx.Value(contextKeyUserID).(string)
		gameID := r.PathValue("battleId")
		storyID := r.PathValue("storyId")

		game, ok := s.confirmAsyncParticipant(w, r, gameID, sessionUserID)
		if !ok {
			return
		}
		st := findStory(game, storyID)
		if st == nil {
			s.Failure(w, r, http.StatusNotFound, Errorf(ENOTFOUND, "STORY_NOT_FOUND"))
			return
		}
		if st.Points != "" || st.Skipped {
			s.Failure(w, r, http.StatusBadRequest, Errorf(EINVALID, "STORY_FINALIZED"))
			return
		}

		_, err := s.PokerDataSvc.RetractVote(gameID, sessionUserID, storyID)
		if err != nil {
			s.Logger.Ctx(ctx).Error("async retract vote", zap.Error(err))
			s.Failure(w, r, http.StatusInternalServerError, err)
			return
		}

		updated, _ := s.PokerDataSvc.GetGameByID(gameID, sessionUserID)
		s.Success(w, r, http.StatusOK, s.asyncGameVisible(updated, sessionUserID), nil)
	}
}

type asyncCommentRequestBody struct {
	Comment string `json:"comment"`
}

// handleAsyncUpsertComment creates or updates a user's comment on an async story.
//
//	@Summary		Add/Update Async Comment
//	@Tags			poker,async
//	@Security		ApiKeyAuth
//	@Router			/battles/{battleId}/stories/{storyId}/comments [post]
func (s *Service) handleAsyncUpsertComment() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		sessionUserID := ctx.Value(contextKeyUserID).(string)
		gameID := r.PathValue("battleId")
		storyID := r.PathValue("storyId")
		if idErr := validate.Var(gameID, "required,uuid"); idErr != nil {
			s.Failure(w, r, http.StatusBadRequest, Errorf(EINVALID, idErr.Error()))
			return
		}
		if idErr := validate.Var(storyID, "required,uuid"); idErr != nil {
			s.Failure(w, r, http.StatusBadRequest, Errorf(EINVALID, idErr.Error()))
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			s.Failure(w, r, http.StatusBadRequest, Errorf(EINVALID, err.Error()))
			return
		}
		var rb asyncCommentRequestBody
		if err := json.Unmarshal(body, &rb); err != nil {
			s.Failure(w, r, http.StatusBadRequest, Errorf(EINVALID, err.Error()))
			return
		}

		game, ok := s.confirmAsyncParticipant(w, r, gameID, sessionUserID)
		if !ok {
			return
		}
		st := findStory(game, storyID)
		if st == nil {
			s.Failure(w, r, http.StatusNotFound, Errorf(ENOTFOUND, "STORY_NOT_FOUND"))
			return
		}
		if st.Points != "" || st.Skipped {
			s.Failure(w, r, http.StatusBadRequest, Errorf(EINVALID, "STORY_FINALIZED"))
			return
		}

		c, err := s.PokerDataSvc.UpsertStoryComment(gameID, storyID, sessionUserID, rb.Comment)
		if err != nil {
			s.Logger.Ctx(ctx).Error("async upsert comment", zap.Error(err))
			s.Failure(w, r, http.StatusInternalServerError, err)
			return
		}
		s.Success(w, r, http.StatusOK, c, nil)
	}
}

// handleAsyncDeleteComment deletes an async comment.
//
//	@Summary		Delete Async Comment
//	@Tags			poker,async
//	@Security		ApiKeyAuth
//	@Router			/battles/{battleId}/stories/{storyId}/comments/{commentId} [delete]
func (s *Service) handleAsyncDeleteComment() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		sessionUserID := ctx.Value(contextKeyUserID).(string)
		gameID := r.PathValue("battleId")
		commentID := r.PathValue("commentId")
		if idErr := validate.Var(gameID, "required,uuid"); idErr != nil {
			s.Failure(w, r, http.StatusBadRequest, Errorf(EINVALID, idErr.Error()))
			return
		}
		if idErr := validate.Var(commentID, "required,uuid"); idErr != nil {
			s.Failure(w, r, http.StatusBadRequest, Errorf(EINVALID, idErr.Error()))
			return
		}

		game, ok := s.confirmAsyncParticipant(w, r, gameID, sessionUserID)
		if !ok {
			return
		}

		existing, err := s.PokerDataSvc.GetStoryCommentByID(commentID)
		if err != nil || existing == nil {
			s.Failure(w, r, http.StatusNotFound, Errorf(ENOTFOUND, "COMMENT_NOT_FOUND"))
			return
		}
		if existing.PokerID != gameID {
			s.Failure(w, r, http.StatusBadRequest, Errorf(EINVALID, "COMMENT_GAME_MISMATCH"))
			return
		}

		// allow author or facilitator only
		if existing.UserID != sessionUserID && !isFacilitatorByID(game, sessionUserID) {
			s.Failure(w, r, http.StatusForbidden, Errorf(EUNAUTHORIZED, "REQUIRES_FACILITATOR"))
			return
		}

		if err := s.PokerDataSvc.DeleteStoryComment(gameID, commentID); err != nil {
			s.Logger.Ctx(ctx).Error("async delete comment", zap.Error(err))
			s.Failure(w, r, http.StatusInternalServerError, err)
			return
		}

		s.Success(w, r, http.StatusOK, nil, nil)
	}
}

type asyncFinalizeRequestBody struct {
	Points string `json:"points" validate:"required"`
}

// handleAsyncFinalizeStory finalizes the points for an async story (facilitator only).
//
//	@Summary		Finalize Async Story
//	@Tags			poker,async
//	@Security		ApiKeyAuth
//	@Router			/battles/{battleId}/stories/{storyId}/finalize [post]
func (s *Service) handleAsyncFinalizeStory() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		sessionUserID := ctx.Value(contextKeyUserID).(string)
		gameID := r.PathValue("battleId")
		storyID := r.PathValue("storyId")

		game, ok := s.confirmAsyncParticipant(w, r, gameID, sessionUserID)
		if !ok {
			return
		}
		if !isFacilitatorByID(game, sessionUserID) {
			s.Failure(w, r, http.StatusForbidden, Errorf(EUNAUTHORIZED, "REQUIRES_FACILITATOR"))
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			s.Failure(w, r, http.StatusBadRequest, Errorf(EINVALID, err.Error()))
			return
		}
		var rb asyncFinalizeRequestBody
		if err := json.Unmarshal(body, &rb); err != nil {
			s.Failure(w, r, http.StatusBadRequest, Errorf(EINVALID, err.Error()))
			return
		}
		if err := validate.Struct(rb); err != nil {
			s.Failure(w, r, http.StatusBadRequest, Errorf(EINVALID, err.Error()))
			return
		}

		if _, err := s.PokerDataSvc.FinalizeStory(gameID, storyID, rb.Points); err != nil {
			s.Logger.Ctx(ctx).Error("async finalize story", zap.Error(err))
			s.Failure(w, r, http.StatusInternalServerError, err)
			return
		}

		updated, _ := s.PokerDataSvc.GetGameByID(gameID, sessionUserID)

		// Best-effort: push the finalized points back to the linked Jira issue
		// using the facilitator's Jira credentials. Fail-open — any error is
		// logged and the local finalize remains successful.
		if updated != nil {
			for _, st := range updated.Stories {
				if st != nil && st.ID == storyID {
					s.pushPointsToJira(ctx, sessionUserID, st, rb.Points)
					break
				}
			}
		}

		s.Success(w, r, http.StatusOK, s.asyncGameVisible(updated, sessionUserID), nil)
	}
}

// handleAsyncReopenStory reopens a finalized async story (facilitator only).
//
//	@Summary		Reopen Async Story
//	@Tags			poker,async
//	@Security		ApiKeyAuth
//	@Router			/battles/{battleId}/stories/{storyId}/reopen [post]
func (s *Service) handleAsyncReopenStory() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		sessionUserID := ctx.Value(contextKeyUserID).(string)
		gameID := r.PathValue("battleId")
		storyID := r.PathValue("storyId")

		game, ok := s.confirmAsyncParticipant(w, r, gameID, sessionUserID)
		if !ok {
			return
		}
		if !isFacilitatorByID(game, sessionUserID) {
			s.Failure(w, r, http.StatusForbidden, Errorf(EUNAUTHORIZED, "REQUIRES_FACILITATOR"))
			return
		}

		if _, err := s.PokerDataSvc.ReopenStory(gameID, storyID); err != nil {
			s.Logger.Ctx(ctx).Error("async reopen story", zap.Error(err))
			s.Failure(w, r, http.StatusInternalServerError, err)
			return
		}

		updated, _ := s.PokerDataSvc.GetGameByID(gameID, sessionUserID)
		s.Success(w, r, http.StatusOK, s.asyncGameVisible(updated, sessionUserID), nil)
	}
}
