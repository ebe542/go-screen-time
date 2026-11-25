// Package bonusmessage validates signed and encrypted offline bonus messages.
package bonusmessage

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ProtonMail/gopenpgp/v3/crypto"
	"github.com/ProtonMail/gopenpgp/v3/profile"

	"github.com/ebe542/go-screen-time/internal/usage"
)

const (
	bonusMessageFormat = "go-screen-time/bonus/v1"
	maxArmoredSize     = 64 * 1024
	maxPayloadSize     = 4 * 1024
	maxBonusMinutes    = (1<<63 - 1) / int64(time.Minute)
)

type bonusMessage struct {
	Format    string    `json:"format"`
	GrantID   string    `json:"grant_id"`
	PCID      string    `json:"pc_id"`
	ChildID   string    `json:"child_id"`
	AppID     string    `json:"app_id,omitempty"`
	Minutes   int64     `json:"minutes"`
	ExpiresAt time.Time `json:"expires_at"`
}

// VerifyBonusMessage verifies a signed OpenPGP message for the given PC and
// authenticated child. The caller supplies the trusted parent verification keys
// and passes a successful grant to the store for one-time redemption.
func VerifyBonusMessage(
	armored string,
	decryptionKey *crypto.Key,
	trustedParents []*crypto.Key,
	pcID, childID string,
	at time.Time,
) (usage.VerifiedBonusGrant, error) {
	if len(armored) == 0 || len(armored) > maxArmoredSize {
		return usage.VerifiedBonusGrant{}, errors.New("invalid armored bonus message size")
	}
	if decryptionKey == nil || len(trustedParents) == 0 || pcID == "" || childID == "" || at.IsZero() {
		return usage.VerifiedBonusGrant{}, errors.New("missing bonus message verification context")
	}

	builder := crypto.PGPWithProfile(profile.RFC9580()).Decryption().
		DecryptionKey(decryptionKey).
		VerifyTime(at.Unix()).
		MaxDecompressedMessageSize(maxPayloadSize)
	for _, parent := range trustedParents {
		if parent == nil {
			return usage.VerifiedBonusGrant{}, errors.New("missing parent verification key")
		}
		builder.VerificationKey(parent)
		if err := builder.Error(); err != nil {
			return usage.VerifiedBonusGrant{}, fmt.Errorf("add parent verification key: %w", err)
		}
	}
	decryptor, err := builder.New()
	if err != nil {
		return usage.VerifiedBonusGrant{}, fmt.Errorf("configure bonus message decryption: %w", err)
	}
	decrypted, err := decryptor.Decrypt([]byte(strings.TrimSpace(armored)), crypto.Armor)
	if err != nil {
		return usage.VerifiedBonusGrant{}, fmt.Errorf("decrypt bonus message: %w", err)
	}
	if err := decrypted.SignatureError(); err != nil {
		return usage.VerifiedBonusGrant{}, fmt.Errorf("verify bonus message signature: %w", err)
	}

	payload := decrypted.Bytes()
	if len(payload) > maxPayloadSize {
		return usage.VerifiedBonusGrant{}, errors.New("bonus message payload is too large")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var message bonusMessage
	if err := decoder.Decode(&message); err != nil {
		return usage.VerifiedBonusGrant{}, fmt.Errorf("decode bonus message: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return usage.VerifiedBonusGrant{}, errors.New("unexpected data after bonus message")
	}

	if message.Format != bonusMessageFormat {
		return usage.VerifiedBonusGrant{}, errors.New("unsupported bonus message format")
	}
	if message.PCID != pcID || message.ChildID != childID {
		return usage.VerifiedBonusGrant{}, errors.New("bonus message is for a different PC or child")
	}
	if strings.TrimSpace(message.GrantID) == "" {
		return usage.VerifiedBonusGrant{}, errors.New("missing bonus grant ID")
	}
	if message.Minutes <= 0 || message.Minutes > maxBonusMinutes {
		return usage.VerifiedBonusGrant{}, errors.New("invalid bonus duration")
	}
	_, offset := message.ExpiresAt.Zone()
	if offset != 0 || message.ExpiresAt.IsZero() || !at.Before(message.ExpiresAt) {
		return usage.VerifiedBonusGrant{}, errors.New("invalid or expired bonus message deadline")
	}

	return usage.VerifiedBonusGrant{
		ID:       message.GrantID,
		ChildID:  message.ChildID,
		AppID:    message.AppID,
		Duration: time.Duration(message.Minutes) * time.Minute,
	}, nil
}
