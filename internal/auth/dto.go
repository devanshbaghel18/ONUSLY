package auth

type GoogleLoginRequest struct {
	IDToken string `json:"idToken"`
}

type GoogleLoginResponse struct {
	User  User   `json:"user"`
	Token string `json:"token"`
}
