package crypto_test

import (
	"strings"
	"testing"

	"github.com/FlameInTheDark/emerald/internal/crypto"
)

func TestEncryptor_EncryptDecrypt(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		plaintext string
		wantErr   bool
	}{
		{
			name:      "standard string encryption",
			key:       "test-key-32-bytes-long!!12345678",
			plaintext: "secret-api-token",
			wantErr:   false,
		},
		{
			name:      "empty string encryption",
			key:       "test-key-32-bytes-long!!12345678",
			plaintext: "",
			wantErr:   false,
		},
		{
			name:      "long string encryption",
			key:       "test-key-32-bytes-long!!12345678",
			plaintext: strings.Repeat("a", 1000),
			wantErr:   false,
		},
		{
			name:      "special characters",
			key:       "test-key-32-bytes-long!!12345678",
			plaintext: "!@#$%^&*()_+-=[]{}|;':\",./<>?",
			wantErr:   false,
		},
		{
			name:      "invalid key length",
			key:       "short-key",
			plaintext: "secret",
			wantErr:   true,
		},
		{
			name:      "key too long",
			key:       "this-key-is-way-too-long-for-aes-gcm-encryption!!",
			plaintext: "secret",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encryptor, err := crypto.NewEncryptor(tt.key)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for key length %d, got nil", len(tt.key))
				}
				return
			}
			if err != nil {
				t.Fatalf("NewEncryptor(%q) error = %v", tt.key, err)
			}

			encrypted, err := encryptor.Encrypt(tt.plaintext)
			if err != nil {
				t.Fatalf("Encrypt() error = %v", err)
			}

			if encrypted == tt.plaintext {
				t.Errorf("encrypted output should differ from plaintext")
			}

			decrypted, err := encryptor.Decrypt(encrypted)
			if err != nil {
				t.Fatalf("Decrypt() error = %v", err)
			}

			if decrypted != tt.plaintext {
				t.Errorf("Decrypt() = %q, want %q", decrypted, tt.plaintext)
			}
		})
	}
}

func TestEncryptor_DifferentCiphertextEachTime(t *testing.T) {
	encryptor, err := crypto.NewEncryptor("test-key-32-bytes-long!!12345678")
	if err != nil {
		t.Fatalf("NewEncryptor() error = %v", err)
	}

	encrypted1, err := encryptor.Encrypt("same-plaintext")
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	encrypted2, err := encryptor.Encrypt("same-plaintext")
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	if encrypted1 == encrypted2 {
		t.Errorf("encrypting the same plaintext should produce different ciphertext due to random nonce")
	}

	decrypted1, err := encryptor.Decrypt(encrypted1)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}

	decrypted2, err := encryptor.Decrypt(encrypted2)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}

	if decrypted1 != decrypted2 {
		t.Errorf("decrypted values should match: %q != %q", decrypted1, decrypted2)
	}
}

func TestEncryptor_DecryptCompatSupportsLegacyFormats(t *testing.T) {
	encryptor, err := crypto.NewEncryptor("test-key-32-bytes-long!!12345678")
	if err != nil {
		t.Fatalf("NewEncryptor() error = %v", err)
	}

	encrypted, err := encryptor.Encrypt("secret-value")
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "prefixed ciphertext",
			input: encrypted,
			want:  "secret-value",
		},
		{
			name:  "legacy unprefixed ciphertext",
			input: strings.TrimPrefix(encrypted, "enc:"),
			want:  "secret-value",
		},
		{
			name:  "legacy plaintext",
			input: `{"botToken":"abc123"}`,
			want:  `{"botToken":"abc123"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := encryptor.DecryptCompat(tt.input)
			if err != nil {
				t.Fatalf("DecryptCompat() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("DecryptCompat() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEncryptor_DecryptInvalidData(t *testing.T) {
	encryptor, err := crypto.NewEncryptor("test-key-32-bytes-long!!12345678")
	if err != nil {
		t.Fatalf("NewEncryptor() error = %v", err)
	}

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "invalid base64",
			input:   "not-valid-base64!@#",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "too short ciphertext",
			input:   "YWJj",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := encryptor.Decrypt(tt.input)
			if tt.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func BenchmarkEncryptor_Encrypt(b *testing.B) {
	encryptor, err := crypto.NewEncryptor("test-key-32-bytes-long!!12345678")
	if err != nil {
		b.Fatalf("NewEncryptor() error = %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := encryptor.Encrypt("benchmark-test-secret")
		if err != nil {
			b.Fatalf("Encrypt() error = %v", err)
		}
	}
}

func BenchmarkEncryptor_Decrypt(b *testing.B) {
	encryptor, err := crypto.NewEncryptor("test-key-32-bytes-long!!12345678")
	if err != nil {
		b.Fatalf("NewEncryptor() error = %v", err)
	}

	encrypted, err := encryptor.Encrypt("benchmark-test-secret")
	if err != nil {
		b.Fatalf("Encrypt() error = %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := encryptor.Decrypt(encrypted)
		if err != nil {
			b.Fatalf("Decrypt() error = %v", err)
		}
	}
}
