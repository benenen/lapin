package database

import "context"

func AssignRoleByCode(ctx context.Context, db DBTX, userID int64, roleCode string) error {
	_, err := db.Exec(ctx, `
		INSERT INTO user_roles (user_id, role_id)
		SELECT $1, id FROM roles WHERE code = $2
		ON CONFLICT (user_id, role_id) DO NOTHING
	`, userID, roleCode)
	return err
}

func ListRoleCodesForUser(ctx context.Context, db DBTX, userID int64) ([]string, error) {
	rows, err := db.Query(ctx, `
		SELECT r.code FROM user_roles ur
		JOIN roles r ON r.id = ur.role_id
		WHERE ur.user_id = $1 ORDER BY r.code
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	roles := make([]string, 0)
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		roles = append(roles, code)
	}
	return roles, rows.Err()
}
