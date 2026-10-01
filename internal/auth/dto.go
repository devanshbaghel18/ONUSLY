package auth

type GoogleLoginRequest struct {
	IDToken    string `json:"idToken"`
	Credential string `json:"credential"`
}

type GoogleLoginResponse struct {
	User  User   `json:"user"`
	Token string `json:"token"`
}
