package auth

type User struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	Picture   string `json:"picture"`
	CreatedAt string `json:"createdAt"`
}

type GoogleLoginRequest struct {
	IDToken string `json:"idToken"`
}

type GoogleLoginResponse struct {
	User  User   `json:"user"`
	Token string `json:"token"`
}