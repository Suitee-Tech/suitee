package credential

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Envelope criptográfico das credenciais em repouso (P1-14):
// AES-256-GCM com chave de aplicação (CLICKSIGN_CREDENTIALS_ENC_KEY).
// O formato no banco é "v1:<nonce b64>:<ciphertext b64>" — versionado para
// permitir trocar o esquema sem re-criptografar tudo de uma vez.
const cryptoVersion = "v1"

var (
	// ErrCredentialCryptoKeyMissing marca ambiente sem a chave de
	// criptografia — tenant com credencial persistida fica fail-closed.
	ErrCredentialCryptoKeyMissing = errors.New("CLICKSIGN_CREDENTIALS_ENC_KEY not configured")
	// ErrCredentialCryptoInvalid marca segredo corrompido/versão desconhecida.
	ErrCredentialCryptoInvalid = errors.New("clicksign credential ciphertext is invalid")
)

// KeyFromHex converte a chave do ambiente (64 hex = 32 bytes) em material
// para AES-256. Vazia/curta → erro tipado.
func KeyFromHex(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, ErrCredentialCryptoKeyMissing
	}
	key, err := hex.DecodeString(raw)
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("%w: enc key must be 64 hex chars (32 bytes)", ErrCredentialCryptoKeyMissing)
	}
	return key, nil
}

// Encrypt cifra um segredo com AES-GCM e devolve o envelope textual.
func Encrypt(key []byte, plaintext string) (string, error) {
	if len(key) != 32 {
		return "", fmt.Errorf("%w: key must be 32 bytes", ErrCredentialCryptoKeyMissing)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	return cryptoVersion + ":" +
		base64.StdEncoding.EncodeToString(nonce) + ":" +
		base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt abre o envelope textual de volta ao segredo original.
func Decrypt(key []byte, envelope string) (string, error) {
	parts := strings.Split(envelope, ":")
	if len(parts) != 3 || parts[0] != cryptoVersion {
		return "", fmt.Errorf("%w: unsupported envelope", ErrCredentialCryptoInvalid)
	}
	nonce, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("%w: bad nonce", ErrCredentialCryptoInvalid)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return "", fmt.Errorf("%w: bad ciphertext", ErrCredentialCryptoInvalid)
	}
	if len(key) != 32 {
		return "", fmt.Errorf("%w: key must be 32 bytes", ErrCredentialCryptoKeyMissing)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("%w: gcm open failed", ErrCredentialCryptoInvalid)
	}
	return string(plaintext), nil
}
