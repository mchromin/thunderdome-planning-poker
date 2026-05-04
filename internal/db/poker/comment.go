package poker

import (
	"fmt"

	"github.com/StevenWeathers/thunderdome-planning-poker/thunderdome"
	"go.uber.org/zap"
)

// UpsertStoryComment inserts or updates a user's comment on a poker story.
// There is at most one comment per (story, user) pair.
func (d *Service) UpsertStoryComment(pokerID string, storyID string, userID string, comment string) (*thunderdome.PokerStoryComment, error) {
	sanitized := d.HTMLSanitizerPolicy.Sanitize(comment)
	c := &thunderdome.PokerStoryComment{
		PokerID: pokerID,
		StoryID: storyID,
		UserID:  userID,
		Comment: sanitized,
	}

	err := d.DB.QueryRow(
		`INSERT INTO thunderdome.poker_story_comment (poker_id, story_id, user_id, comment)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (story_id, user_id) DO UPDATE
		 SET comment = EXCLUDED.comment, updated_date = NOW()
		 RETURNING id, created_date, updated_date`,
		pokerID, storyID, userID, sanitized,
	).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		d.Logger.Error("upsert poker story comment error", zap.Error(err),
			zap.String("PokerID", pokerID), zap.String("StoryID", storyID),
			zap.String("UserID", userID))
		return nil, fmt.Errorf("upsert poker story comment error: %v", err)
	}

	// fetch user name for response
	_ = d.DB.QueryRow(`SELECT COALESCE(name, '') FROM thunderdome.users WHERE id = $1`, userID).Scan(&c.UserName)

	return c, nil
}

// DeleteStoryComment removes a comment by ID. Caller must enforce that the
// requesting user is the author or a facilitator.
func (d *Service) DeleteStoryComment(pokerID string, commentID string) error {
	if _, err := d.DB.Exec(
		`DELETE FROM thunderdome.poker_story_comment WHERE id = $1 AND poker_id = $2`,
		commentID, pokerID,
	); err != nil {
		d.Logger.Error("delete poker story comment error", zap.Error(err),
			zap.String("PokerID", pokerID), zap.String("CommentID", commentID))
		return fmt.Errorf("delete poker story comment error: %v", err)
	}
	return nil
}

// GetStoryCommentByID looks up a comment by ID (for ownership checks).
func (d *Service) GetStoryCommentByID(commentID string) (*thunderdome.PokerStoryComment, error) {
	c := &thunderdome.PokerStoryComment{}
	err := d.DB.QueryRow(
		`SELECT id, poker_id, story_id, user_id, comment, created_date, updated_date
		 FROM thunderdome.poker_story_comment WHERE id = $1`,
		commentID,
	).Scan(&c.ID, &c.PokerID, &c.StoryID, &c.UserID, &c.Comment, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get poker story comment error: %v", err)
	}
	return c, nil
}

// GetStoryComments returns all comments for a single story joined with the
// author's display name.
func (d *Service) GetStoryComments(storyID string) ([]*thunderdome.PokerStoryComment, error) {
	comments := make([]*thunderdome.PokerStoryComment, 0)
	rows, err := d.DB.Query(
		`SELECT c.id, c.poker_id, c.story_id, c.user_id, COALESCE(u.name, ''), c.comment,
		        c.created_date, c.updated_date
		 FROM thunderdome.poker_story_comment c
		 LEFT JOIN thunderdome.users u ON u.id = c.user_id
		 WHERE c.story_id = $1
		 ORDER BY c.created_date ASC`,
		storyID,
	)
	if err != nil {
		return nil, fmt.Errorf("get poker story comments error: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		c := &thunderdome.PokerStoryComment{}
		if err := rows.Scan(&c.ID, &c.PokerID, &c.StoryID, &c.UserID, &c.UserName,
			&c.Comment, &c.CreatedAt, &c.UpdatedAt); err != nil {
			d.Logger.Error("scan poker story comment error", zap.Error(err))
			continue
		}
		comments = append(comments, c)
	}
	return comments, nil
}

// GetGameComments returns all comments for all stories of a poker game.
func (d *Service) GetGameComments(pokerID string) ([]*thunderdome.PokerStoryComment, error) {
	comments := make([]*thunderdome.PokerStoryComment, 0)
	rows, err := d.DB.Query(
		`SELECT c.id, c.poker_id, c.story_id, c.user_id, COALESCE(u.name, ''), c.comment,
		        c.created_date, c.updated_date
		 FROM thunderdome.poker_story_comment c
		 LEFT JOIN thunderdome.users u ON u.id = c.user_id
		 WHERE c.poker_id = $1
		 ORDER BY c.created_date ASC`,
		pokerID,
	)
	if err != nil {
		return nil, fmt.Errorf("get poker game comments error: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		c := &thunderdome.PokerStoryComment{}
		if err := rows.Scan(&c.ID, &c.PokerID, &c.StoryID, &c.UserID, &c.UserName,
			&c.Comment, &c.CreatedAt, &c.UpdatedAt); err != nil {
			d.Logger.Error("scan poker game comment error", zap.Error(err))
			continue
		}
		comments = append(comments, c)
	}
	return comments, nil
}
