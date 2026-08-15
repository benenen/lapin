package database

import (
	"context"
	"time"
)

type Configuration struct {
	ID        int64
	Key1      string
	Key2      string
	Key3      string
	Value     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func InsertConfiguration(ctx context.Context, db DBTX, configuration Configuration) (int64, error) {
	var id int64
	err := db.QueryRow(ctx, `
		INSERT INTO configurations (key1, key2, key3, value)
		VALUES ($1, $2, $3, $4) RETURNING id
	`, configuration.Key1, configuration.Key2, configuration.Key3, configuration.Value).Scan(&id)
	return id, err
}

func FindConfigurationByID(ctx context.Context, db DBTX, id int64) (Configuration, error) {
	var configuration Configuration
	err := db.QueryRow(ctx, `
		SELECT id, key1, key2, key3, value, created_at, updated_at
		FROM configurations WHERE id = $1
	`, id).Scan(&configuration.ID, &configuration.Key1, &configuration.Key2, &configuration.Key3, &configuration.Value, &configuration.CreatedAt, &configuration.UpdatedAt)
	return configuration, err
}

func ListConfigurationsByKeys(ctx context.Context, db DBTX, key1, key2, key3 string) ([]Configuration, error) {
	rows, err := db.Query(ctx, `
		SELECT id, key1, key2, key3, value, created_at, updated_at
		FROM configurations WHERE key1 = $1 AND key2 = $2 AND key3 = $3 ORDER BY id
	`, key1, key2, key3)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Configuration, 0)
	for rows.Next() {
		var item Configuration
		if err := rows.Scan(&item.ID, &item.Key1, &item.Key2, &item.Key3, &item.Value, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
