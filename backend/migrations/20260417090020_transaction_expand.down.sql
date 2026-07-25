ALTER TABLE transactions
    DROP COLUMN IF EXISTS currency,
    DROP COLUMN IF EXISTS debit_credit,
    DROP COLUMN IF EXISTS merchant_name,
    DROP COLUMN IF EXISTS merchant_category,
    DROP COLUMN IF EXISTS description,
    DROP COLUMN IF EXISTS assigned_category,
    DROP COLUMN IF EXISTS user_notes,
    DROP COLUMN IF EXISTS linked_account,
    DROP COLUMN IF EXISTS payment_intent_id,
    DROP COLUMN IF EXISTS upi_intent_id,
    DROP COLUMN IF EXISTS bank_reference;
