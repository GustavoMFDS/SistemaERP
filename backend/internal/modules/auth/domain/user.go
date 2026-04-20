package domain

type User struct {
	ID           string
	Email        string
	Name         string
	PasswordHash string
	Active       bool
}
