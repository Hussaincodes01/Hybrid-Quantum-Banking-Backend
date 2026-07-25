ALTER TABLE goals
    DROP COLUMN IF EXISTS description,
    DROP COLUMN IF EXISTS currency,
    DROP COLUMN IF EXISTS monthly_contribution_paise,
    DROP COLUMN IF EXISTS frequency,
    DROP COLUMN IF EXISTS priority,
    DROP COLUMN IF EXISTS start_date,
    DROP COLUMN IF EXISTS linked_account;
