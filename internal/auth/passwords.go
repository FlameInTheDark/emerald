package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	passwordHashPrefix     = "$argon2id$"
	passwordHashVersionTag = "v=19"
	passwordSaltLength     = 16
	passwordKeyLength      = 32
	passwordMemoryKB       = 64 * 1024
	passwordIterations     = 3
	passwordParallelism    = 2
)

func HashPassword(password string) (string, error) {
	salt := make([]byte, passwordSaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}

	hash := argon2.IDKey(
		[]byte(password),
		salt,
		passwordIterations,
		passwordMemoryKB,
		passwordParallelism,
		passwordKeyLength,
	)

	return fmt.Sprintf(
		"%s%s$m=%d,t=%d,p=%d$%s$%s",
		passwordHashPrefix,
		passwordHashVersionTag,
		passwordMemoryKB,
		passwordIterations,
		passwordParallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

func VerifyPasswordHash(stored string, password string) (bool, error) {
	params, salt, expectedHash, err := parsePasswordHash(stored)
	if err != nil {
		return false, err
	}

	computed := argon2.IDKey(
		[]byte(password),
		salt,
		params.iterations,
		params.memoryKB,
		params.parallelism,
		uint32(len(expectedHash)),
	)

	return subtle.ConstantTimeCompare(computed, expectedHash) == 1, nil
}

func IsPasswordHash(value string) bool {
	return strings.HasPrefix(strings.TrimSpace(value), passwordHashPrefix)
}

type passwordHashParams struct {
	memoryKB    uint32
	iterations  uint32
	parallelism uint8
}

func parsePasswordHash(stored string) (passwordHashParams, []byte, []byte, error) {
	parts := strings.Split(strings.TrimSpace(stored), "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return passwordHashParams{}, nil, nil, fmt.Errorf("unsupported password hash format")
	}
	if parts[2] != passwordHashVersionTag {
		return passwordHashParams{}, nil, nil, fmt.Errorf("unsupported password hash version")
	}

	params, err := parsePasswordHashParams(parts[3])
	if err != nil {
		return passwordHashParams{}, nil, nil, err
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return passwordHashParams{}, nil, nil, fmt.Errorf("decode password salt: %w", err)
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return passwordHashParams{}, nil, nil, fmt.Errorf("decode password hash: %w", err)
	}
	if len(hash) == 0 {
		return passwordHashParams{}, nil, nil, fmt.Errorf("password hash is empty")
	}

	return params, salt, hash, nil
}

func parsePasswordHashParams(raw string) (passwordHashParams, error) {
	var params passwordHashParams
	for _, part := range strings.Split(raw, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			return passwordHashParams{}, fmt.Errorf("invalid password hash parameters")
		}

		parsed, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return passwordHashParams{}, fmt.Errorf("parse password hash parameter %s: %w", key, err)
		}

		switch key {
		case "m":
			params.memoryKB = uint32(parsed)
		case "t":
			params.iterations = uint32(parsed)
		case "p":
			params.parallelism = uint8(parsed)
		default:
			return passwordHashParams{}, fmt.Errorf("unsupported password hash parameter %q", key)
		}
	}

	if params.memoryKB == 0 || params.iterations == 0 || params.parallelism == 0 {
		return passwordHashParams{}, fmt.Errorf("password hash parameters are incomplete")
	}

	return params, nil
}
