package database

import (
	"context"
	"time"
)

type Role struct {
	ID        int64
	Code      string
	Name      string
	CreatedAt time.Time
}

func ListRoles(ctx context.Context, db DBTX) ([]Role, error) {
	rows, err := db.Query(ctx, `SELECT id, code, name, created_at FROM roles ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	roles := make([]Role, 0)
	for rows.Next() {
		var role Role
		if err := rows.Scan(&role.ID, &role.Code, &role.Name, &role.CreatedAt); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}
