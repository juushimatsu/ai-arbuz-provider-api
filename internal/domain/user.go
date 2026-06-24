package domain

// User — the single admin account (§4.12).
type User struct {
	ID           ID
	Login        string
	PasswordHash string // bcrypt
	CreatedAt    Time
	UpdatedAt    Time
}

// Session — admin session token (revocable, stored).
type Session struct {
	Token     string
	UserID    ID
	CreatedAt Time
	ExpiresAt Time
}
