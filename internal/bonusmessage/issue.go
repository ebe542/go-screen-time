package bonusmessage

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ProtonMail/gopenpgp/v3/crypto"
	"github.com/ProtonMail/gopenpgp/v3/profile"
)

// BonusRequest specifies the recipient, scope, duration, and redemption deadline.
type BonusRequest struct {
	PCID      string
	ChildID   string
	AppID     string
	Minutes   int64
	ExpiresAt time.Time
}

// IssueBonusMessage signs and encrypts a new one-time bonus grant for the PC.
func IssueBonusMessage(
	recipient, signer *crypto.Key,
	request BonusRequest,
	at time.Time,
) (string, error) {
	if recipient == nil || signer == nil {
		return "", errors.New("bonus message requires a recipient key and a signing key")
	}
	if strings.TrimSpace(request.PCID) == "" || strings.TrimSpace(request.ChildID) == "" {
		return "", errors.New("bonus message requires a target PC and a child account")
	}
	if request.Minutes <= 0 || request.Minutes > maxBonusMinutes {
		return "", errors.New("invalid bonus duration")
	}
	if at.IsZero() || request.ExpiresAt.IsZero() || !request.ExpiresAt.After(at) {
		return "", errors.New("bonus message deadline must be after issuance")
	}

	payload, err := json.Marshal(bonusMessage{
		Format:    bonusMessageFormat,
		GrantID:   rand.Text(),
		PCID:      request.PCID,
		ChildID:   request.ChildID,
		AppID:     request.AppID,
		Minutes:   request.Minutes,
		ExpiresAt: request.ExpiresAt.UTC(),
	})
	if err != nil {
		return "", fmt.Errorf("encode bonus message: %w", err)
	}
	if len(payload) > maxPayloadSize {
		return "", errors.New("bonus message payload is too large")
	}

	encryptor, err := crypto.PGPWithProfile(profile.RFC9580()).Encryption().
		Recipient(recipient).
		SigningKey(signer).
		EncryptionTime(at.Unix()).
		New()
	if err != nil {
		return "", fmt.Errorf("configure bonus message encryption: %w", err)
	}
	message, err := encryptor.Encrypt(payload)
	if err != nil {
		return "", fmt.Errorf("sign and encrypt bonus message: %w", err)
	}
	armored, err := message.ArmorBytes()
	if err != nil {
		return "", fmt.Errorf("encode armored bonus message: %w", err)
	}
	if len(armored) > maxArmoredSize {
		return "", errors.New("armored bonus message is too large")
	}

	return string(armored), nil
}
