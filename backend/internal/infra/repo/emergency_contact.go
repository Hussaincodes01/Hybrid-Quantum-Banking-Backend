package repo

import (
	"context"
	"fmt"
)

type EmergencyContactRow struct {
	ID           string
	UserID       string
	Name         string
	Relationship string
	Phone        string
	Email        string
}

func (r *Repo) CreateEmergencyContact(ctx context.Context, c EmergencyContactRow, userID string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx,
		`INSERT INTO emergency_contacts (user_id, name, relationship, phone, email)
		 VALUES ($1,$2,$3,$4,$5) RETURNING id`,
		userID, c.Name, c.Relationship, c.Phone, c.Email,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("create emergency contact: %w", err)
	}
	return id, nil
}

func (r *Repo) ListEmergencyContactsByUser(ctx context.Context, userID string) ([]EmergencyContactRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, name, relationship, phone, email
		 FROM emergency_contacts WHERE user_id = $1`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list emergency contacts: %w", err)
	}
	defer rows.Close()
	var contacts []EmergencyContactRow
	for rows.Next() {
		var c EmergencyContactRow
		if err := rows.Scan(&c.ID, &c.UserID, &c.Name, &c.Relationship, &c.Phone, &c.Email); err != nil {
			return nil, fmt.Errorf("scan emergency contact: %w", err)
		}
		contacts = append(contacts, c)
	}
	return contacts, rows.Err()
}

func (r *Repo) DeleteEmergencyContact(ctx context.Context, contactID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM emergency_contacts WHERE id = $1`, contactID)
	if err != nil {
		return fmt.Errorf("delete emergency contact: %w", err)
	}
	return nil
}

func (r *Repo) ListAllEmergencyContacts(ctx context.Context) (map[string][]EmergencyContactRow, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, user_id, name, relationship, phone, email FROM emergency_contacts`)
	if err != nil {
		return nil, fmt.Errorf("list all emergency contacts: %w", err)
	}
	defer rows.Close()
	result := make(map[string][]EmergencyContactRow)
	for rows.Next() {
		var c EmergencyContactRow
		if err := rows.Scan(&c.ID, &c.UserID, &c.Name, &c.Relationship, &c.Phone, &c.Email); err != nil {
			return nil, fmt.Errorf("scan emergency contact: %w", err)
		}
		result[c.UserID] = append(result[c.UserID], c)
	}
	return result, rows.Err()
}
