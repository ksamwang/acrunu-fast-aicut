-- name: CreateUser :one
INSERT INTO users (
    username,
    display_name,
    email,
    password_hash,
    role,
    status
) VALUES (
    $1, $2, $3, $4, $5, $6
)
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users
WHERE id = $1;

-- name: GetUserByUsername :one
SELECT * FROM users
WHERE username = $1;

-- name: ListUsers :many
SELECT * FROM users
ORDER BY created_at DESC;

-- name: UpdateUser :one
UPDATE users
SET username = $2,
    display_name = $3,
    email = $4,
    password_hash = $5,
    role = $6,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteUser :execrows
DELETE FROM users
WHERE id = $1;

-- name: CountActiveAdmins :one
SELECT count(*)::integer
FROM users
WHERE role = 'admin'
  AND status = 'active';

-- name: UpdateUserLastLogin :exec
UPDATE users
SET last_login_at = now(),
    updated_at = now()
WHERE id = $1;

-- name: UpdateUserStatus :exec
UPDATE users
SET status = $2,
    updated_at = now()
WHERE id = $1;
