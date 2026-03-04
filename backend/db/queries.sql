-- name: CreateRoom :one
INSERT INTO rooms (id, name) VALUES ($1, $2)
RETURNING id, name, created_at;

-- name: GetRoom :one
SELECT id, name, created_at FROM rooms WHERE id = $1;

-- name: ListRooms :many
SELECT id, name, created_at FROM rooms ORDER BY created_at ASC;

-- name: DeleteRoom :exec
DELETE FROM rooms WHERE id = $1;
