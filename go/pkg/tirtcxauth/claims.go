package tirtcxauth

// TokenClaims is the JSON payload carried by a TiRTC-X WHIP token.
type TokenClaims struct {
	Subject   string `json:"sub"`
	Scope     string `json:"scope"`
	Issuer    string `json:"iss"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
	Nonce     string `json:"nonce"`
}
