package repo

import (
	"context"
	"fmt"
	"time"
)

type UserRow struct {
	ID               string
	Name             string
	Phone            string
	Email            string
	KIN              string
	UBT              string
	EKYCVerified     bool
	BiometricEnabled bool
	DeviceBound      bool
	SIMBound         bool
	NudgePreference  string
	CreatedAt        time.Time
}

func (r *Repo) CreateUser(ctx context.Context, name, phone, email, deviceFingerprint, ubt string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx,
		`INSERT INTO users (name, phone, email, device_fingerprint, ubt)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id`,
		name, phone, email, deviceFingerprint, ubt,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("create user: %w", err)
	}
	return id, nil
}

func (r *Repo) GetUserByPhone(ctx context.Context, phone string) (UserRow, error) {
	var u UserRow
	var kin *string
	err := r.pool.QueryRow(ctx,
		`SELECT id, name, phone, email, kin, ubt, ekyc_verified, biometric_enabled,
		        device_bound, sim_bound, nudge_preference, created_at
		 FROM users WHERE phone = $1`, phone,
	).Scan(&u.ID, &u.Name, &u.Phone, &u.Email, &kin, &u.UBT,
		&u.EKYCVerified, &u.BiometricEnabled, &u.DeviceBound, &u.SIMBound,
		&u.NudgePreference, &u.CreatedAt)
	if err != nil {
		return UserRow{}, fmt.Errorf("get user by phone: %w", err)
	}
	if kin != nil {
		u.KIN = *kin
	}
	return u, nil
}

func (r *Repo) GetUserByID(ctx context.Context, id string) (UserRow, error) {
	var u UserRow
	var kin *string
	err := r.pool.QueryRow(ctx,
		`SELECT id, name, phone, email, kin, ubt, ekyc_verified, biometric_enabled,
		        device_bound, sim_bound, nudge_preference, created_at
		 FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Name, &u.Phone, &u.Email, &kin, &u.UBT,
		&u.EKYCVerified, &u.BiometricEnabled, &u.DeviceBound, &u.SIMBound,
		&u.NudgePreference, &u.CreatedAt)
	if err != nil {
		return UserRow{}, fmt.Errorf("get user by id: %w", err)
	}
	if kin != nil {
		u.KIN = *kin
	}
	return u, nil
}

func (r *Repo) ListAllUsers(ctx context.Context) ([]UserRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, phone, email, kin, ubt, ekyc_verified, biometric_enabled,
		        device_bound, sim_bound, nudge_preference, created_at
		 FROM users ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list all users: %w", err)
	}
	defer rows.Close()

	var users []UserRow
	for rows.Next() {
		var u UserRow
		var kin *string
		if err := rows.Scan(&u.ID, &u.Name, &u.Phone, &u.Email, &kin, &u.UBT,
			&u.EKYCVerified, &u.BiometricEnabled, &u.DeviceBound, &u.SIMBound,
			&u.NudgePreference, &u.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan user row: %w", err)
		}
		if kin != nil {
			u.KIN = *kin
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate users: %w", err)
	}
	return users, nil
}

func (r *Repo) UpdateUserKin(ctx context.Context, id, kin string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE users SET kin = $1, updated_at = NOW() WHERE id = $2`, kin, id,
	)
	if err != nil {
		return fmt.Errorf("update user kin: %w", err)
	}
	return nil
}

func (r *Repo) UpdateUserBiometric(ctx context.Context, id string, enabled bool) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE users SET biometric_enabled = $1, updated_at = NOW() WHERE id = $2`, enabled, id,
	)
	if err != nil {
		return fmt.Errorf("update user biometric: %w", err)
	}
	return nil
}
