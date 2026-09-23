package common

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPasswordEncryptionTransportContract(t *testing.T) {
	passwordEncryptionState.RLock()
	savedPrivate := passwordEncryptionState.privateKey
	savedPublic := passwordEncryptionState.publicKey
	savedID := passwordEncryptionState.keyID
	passwordEncryptionState.RUnlock()
	t.Cleanup(func() {
		passwordEncryptionState.Lock()
		defer passwordEncryptionState.Unlock()
		passwordEncryptionState.privateKey = savedPrivate
		passwordEncryptionState.publicKey = savedPublic
		passwordEncryptionState.keyID = savedID
	})

	privatePEM, err := GeneratePasswordEncryptionPrivateKey()
	require.NoError(t, err)
	require.NoError(t, LoadPasswordEncryptionPrivateKey(privatePEM))
	keyID, publicPEM := PasswordEncryptionPublicKey()
	require.NotEmpty(t, keyID)
	block, rest := pem.Decode([]byte(publicPEM))
	require.NotNil(t, block)
	assert.Equal(t, "PUBLIC KEY", block.Type)
	assert.Empty(t, rest)
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	require.NoError(t, err)
	publicKey, ok := parsed.(*rsa.PublicKey)
	require.True(t, ok)

	const password = "密码 with spaces & symbols!"
	ciphertext, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, publicKey, []byte(password), nil)
	require.NoError(t, err)
	encoded := base64.StdEncoding.EncodeToString(ciphertext)
	decoded, err := DecryptPassword(encoded, keyID)
	require.NoError(t, err)
	assert.Equal(t, password, decoded)

	emptyCiphertext, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, publicKey, nil, nil)
	require.NoError(t, err)
	for _, tc := range []struct {
		name, ciphertext, kid string
	}{
		{"missing key id", encoded, ""},
		{"wrong key id", encoded, "stale-key"},
		{"invalid base64", "%%%", keyID},
		{"wrong ciphertext length", base64.StdEncoding.EncodeToString([]byte("short")), keyID},
		{"invalid OAEP", base64.StdEncoding.EncodeToString(make([]byte, publicKey.Size())), keyID},
		{"empty password", base64.StdEncoding.EncodeToString(emptyCiphertext), keyID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value, decryptErr := DecryptPassword(tc.ciphertext, tc.kid)
			assert.ErrorIs(t, decryptErr, ErrPasswordEncryptionInvalid)
			assert.Empty(t, value)
		})
	}

	// A failed reload must not discard the usable key; loading the same stored
	// key after restart must retain its public identity and decrypt old payloads.
	require.Error(t, LoadPasswordEncryptionPrivateKey("invalid pem"))
	require.Error(t, LoadPasswordEncryptionPrivateKey(privatePEM+"trailing data"))
	activeID, activePEM := PasswordEncryptionPublicKey()
	assert.Equal(t, keyID, activeID)
	assert.Equal(t, publicPEM, activePEM)
	require.NoError(t, LoadPasswordEncryptionPrivateKey(privatePEM))
	activeID, activePEM = PasswordEncryptionPublicKey()
	assert.Equal(t, keyID, activeID)
	assert.Equal(t, publicPEM, activePEM)
	decoded, err = DecryptPassword(encoded, keyID)
	require.NoError(t, err)
	assert.Equal(t, password, decoded)
}
