package paseto

import (
	"strings"

	paseto "aidanwoods.dev/go-paseto"
)

type Mode string

const (
	ModeLocal  Mode = "local"  // v4.local (encrypted)
	ModePublic Mode = "public" // v4.public (signed)
)

type Keys struct {
	Mode Mode

	// v4.local
	Symmetric *paseto.V4SymmetricKey

	// v4.public
	Secret *paseto.V4AsymmetricSecretKey
	Public *paseto.V4AsymmetricPublicKey
}

type KeyStrings struct {
	Mode Mode

	SymmetricHex string

	SecretHex string
	PublicHex string
}

func LoadKeys(in KeyStrings) (Keys, error) {
	switch in.Mode {
	case ModeLocal:
		hex := strings.TrimSpace(in.SymmetricHex)
		if hex == "" {
			return Keys{}, ErrConfig{Msg: "ModeLocal requires SymmetricHex"}
		}
		k, err := paseto.V4SymmetricKeyFromHex(hex)
		if err != nil {
			return Keys{}, ErrConfig{Msg: "invalid symmetric key hex: " + err.Error()}
		}
		return Keys{Mode: ModeLocal, Symmetric: &k}, nil

	case ModePublic:
		secHex := strings.TrimSpace(in.SecretHex)
		pubHex := strings.TrimSpace(in.PublicHex)

		// Allow either:
		// - secret only (we can derive public)
		// - public only (verify-only services)
		// - both (explicit)
		var out Keys
		out.Mode = ModePublic

		if secHex != "" {
			sk, err := paseto.NewV4AsymmetricSecretKeyFromHex(secHex)
			if err != nil {
				return Keys{}, ErrConfig{Msg: "invalid secret key hex: " + err.Error()}
			}
			out.Secret = &sk
			pk := sk.Public()
			out.Public = &pk
		}

		if pubHex != "" {
			pk, err := paseto.NewV4AsymmetricPublicKeyFromHex(pubHex)
			if err != nil {
				return Keys{}, ErrConfig{Msg: "invalid public key hex: " + err.Error()}
			}
			out.Public = &pk
		}

		if out.Public == nil && out.Secret == nil {
			return Keys{}, ErrConfig{Msg: "ModePublic requires SecretHex and/or PublicHex"}
		}
		return out, nil

	default:
		return Keys{}, ErrConfig{Msg: "unknown mode (use local|public)"}
	}
}

func NewLocalKeys() Keys {
	k := paseto.NewV4SymmetricKey()
	return Keys{Mode: ModeLocal, Symmetric: &k}
}

func NewPublicKeys() Keys {
	sk := paseto.NewV4AsymmetricSecretKey()
	pk := sk.Public()
	return Keys{Mode: ModePublic, Secret: &sk, Public: &pk}
}
