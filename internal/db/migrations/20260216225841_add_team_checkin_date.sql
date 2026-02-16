-- +goose Up
-- +goose StatementBegin
ALTER TABLE thunderdome.team_checkin 
ADD COLUMN checkin_date DATE DEFAULT CURRENT_DATE NOT NULL;

-- Backfill existing records to use the date portion of created_date as checkin_date
UPDATE thunderdome.team_checkin 
SET checkin_date = DATE(created_date);

-- Create an index on checkin_date for better query performance
CREATE INDEX IF NOT EXISTS team_checkin_date_idx ON thunderdome.team_checkin USING btree (checkin_date);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS thunderdome.team_checkin_date_idx;
ALTER TABLE thunderdome.team_checkin 
DROP COLUMN checkin_date;
-- +goose StatementEnd
