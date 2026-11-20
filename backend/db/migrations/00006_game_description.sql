-- +goose Up
-- Gift wishes are free text. The ruble amount is no longer a separate field.

ALTER TABLE games DROP CONSTRAINT games_budget_check;
ALTER TABLE games DROP COLUMN budget_rub;
ALTER TABLE games ADD COLUMN description text NULL;
ALTER TABLE games ADD CONSTRAINT games_description_len CHECK (
    description IS NULL OR char_length(description) BETWEEN 1 AND 1000
);

-- +goose Down
ALTER TABLE games DROP CONSTRAINT games_description_len;
ALTER TABLE games DROP COLUMN description;
ALTER TABLE games ADD COLUMN budget_rub integer NULL;
ALTER TABLE games ADD CONSTRAINT games_budget_check CHECK (
    budget_rub IS NULL OR (budget_rub > 0 AND budget_rub <= 100000000)
);
