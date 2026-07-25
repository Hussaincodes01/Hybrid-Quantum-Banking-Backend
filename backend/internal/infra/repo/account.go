package repo

import (
	"context"
	"fmt"
	"time"
)

type AccountRow struct {
	ID                  string
	UserID              string
	AccountHolderName   string
	BankName            string
	Branch              string
	IFSCCode            string
	MaskedAccountNumber string
	UPIID               string
	AccountType         string
	VerificationStatus  string
	Nickname            string
	PrimaryAccountFlag  bool
	AccountToken        string
	TokenProvider       string
	BalancePaise        int64
	LinkedAt            time.Time
}

func (r *Repo) CreateAccount(ctx context.Context, a AccountRow) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx,
		`INSERT INTO accounts (user_id, account_holder_name, bank_name, branch, ifsc_code,
		 masked_account_number, upi_id, account_type, verification_status, nickname,
		 primary_account_flag, account_token, token_provider, balance_paise, linked_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		 RETURNING id`,
		a.UserID, a.AccountHolderName, a.BankName, a.Branch, a.IFSCCode,
		a.MaskedAccountNumber, a.UPIID, a.AccountType, a.VerificationStatus,
		a.Nickname, a.PrimaryAccountFlag, a.AccountToken, a.TokenProvider,
		a.BalancePaise, a.LinkedAt,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("create account: %w", err)
	}
	return id, nil
}

func (r *Repo) ListAccountsByUser(ctx context.Context, userID string) ([]AccountRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, account_holder_name, bank_name, branch, ifsc_code,
		        masked_account_number, upi_id, account_type, verification_status,
		        nickname, primary_account_flag, account_token, token_provider,
		        balance_paise, linked_at
		 FROM accounts WHERE user_id = $1`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list accounts by user: %w", err)
	}
	defer rows.Close()

	var accounts []AccountRow
	for rows.Next() {
		var a AccountRow
		if err := rows.Scan(&a.ID, &a.UserID, &a.AccountHolderName, &a.BankName,
			&a.Branch, &a.IFSCCode, &a.MaskedAccountNumber, &a.UPIID,
			&a.AccountType, &a.VerificationStatus, &a.Nickname,
			&a.PrimaryAccountFlag, &a.AccountToken, &a.TokenProvider,
			&a.BalancePaise, &a.LinkedAt); err != nil {
			return nil, fmt.Errorf("scan account row: %w", err)
		}
		accounts = append(accounts, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate accounts: %w", err)
	}
	return accounts, nil
}

func (r *Repo) GetAccountByID(ctx context.Context, accountID string) (AccountRow, error) {
	var a AccountRow
	err := r.pool.QueryRow(ctx,
		`SELECT id, user_id, account_holder_name, bank_name, branch, ifsc_code,
		        masked_account_number, upi_id, account_type, verification_status,
		        nickname, primary_account_flag, account_token, token_provider,
		        balance_paise, linked_at
		 FROM accounts WHERE id = $1`, accountID,
	).Scan(&a.ID, &a.UserID, &a.AccountHolderName, &a.BankName,
		&a.Branch, &a.IFSCCode, &a.MaskedAccountNumber, &a.UPIID,
		&a.AccountType, &a.VerificationStatus, &a.Nickname,
		&a.PrimaryAccountFlag, &a.AccountToken, &a.TokenProvider,
		&a.BalancePaise, &a.LinkedAt)
	if err != nil {
		return AccountRow{}, fmt.Errorf("get account by id: %w", err)
	}
	return a, nil
}

func (r *Repo) UpdateAccount(ctx context.Context, accountID, nickname string, primaryFlag bool, verificationStatus string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE accounts SET nickname = $1, primary_account_flag = $2, verification_status = $3
		 WHERE id = $4`,
		nickname, primaryFlag, verificationStatus, accountID,
	)
	if err != nil {
		return fmt.Errorf("update account: %w", err)
	}
	return nil
}

func (r *Repo) ListAllAccounts(ctx context.Context) ([]AccountRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, account_holder_name, bank_name, branch, ifsc_code,
		        masked_account_number, upi_id, account_type, verification_status,
		        nickname, primary_account_flag, account_token, token_provider,
		        balance_paise, linked_at
		 FROM accounts ORDER BY linked_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list all accounts: %w", err)
	}
	defer rows.Close()

	var accounts []AccountRow
	for rows.Next() {
		var a AccountRow
		if err := rows.Scan(&a.ID, &a.UserID, &a.AccountHolderName, &a.BankName,
			&a.Branch, &a.IFSCCode, &a.MaskedAccountNumber, &a.UPIID,
			&a.AccountType, &a.VerificationStatus, &a.Nickname,
			&a.PrimaryAccountFlag, &a.AccountToken, &a.TokenProvider,
			&a.BalancePaise, &a.LinkedAt); err != nil {
			return nil, fmt.Errorf("scan account: %w", err)
		}
		accounts = append(accounts, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate accounts: %w", err)
	}
	return accounts, nil
}
