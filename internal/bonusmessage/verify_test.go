package bonusmessage

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ProtonMail/gopenpgp/v3/crypto"
	"github.com/ProtonMail/gopenpgp/v3/profile"

	"github.com/ebe542/go-screen-time/internal/usage"
)

func TestVerifyBonusMessage(t *testing.T) {
	pgp := crypto.PGPWithProfile(profile.RFC9580())
	pcPrivate, pcPublic := testKeyPair(t, pgp, "pc")
	_, otherPCPublic := testKeyPair(t, pgp, "other-pc")
	parentA, parentAPublic := testKeyPair(t, pgp, "parent-a")
	parentB, parentBPublic := testKeyPair(t, pgp, "parent-b")
	unknownParent, _ := testKeyPair(t, pgp, "unknown-parent")
	trusted := []*crypto.Key{parentAPublic, parentBPublic}

	at := time.Now().UTC().Truncate(time.Second).Add(time.Minute)
	expiresAt := at.Add(15 * time.Minute)
	const grantID = "2eaa766d-c0c6-42f1-9e6e-0da9de609f48"

	type message struct {
		Format    string    `json:"format"`
		GrantID   string    `json:"grant_id"`
		PCID      string    `json:"pc_id"`
		ChildID   string    `json:"child_id"`
		AppID     string    `json:"app_id,omitempty"`
		Minutes   int64     `json:"minutes"`
		ExpiresAt time.Time `json:"expires_at"`
	}

	valid := message{
		Format:    "go-screen-time/bonus/v1",
		GrantID:   grantID,
		PCID:      "pc-a",
		ChildID:   "child-a",
		AppID:     "app-a",
		Minutes:   17,
		ExpiresAt: expiresAt,
	}

	tests := []struct {
		name      string
		signer    *crypto.Key
		recipient *crypto.Key
		change    func(*message)
		tamper    bool
		wantErr   bool
	}{
		{name: "first registered parent", signer: parentA},
		{name: "second registered parent", signer: parentB},
		{
			name:   "shared bonus without app",
			signer: parentA,
			change: func(m *message) { m.AppID = "" },
		},
		{
			name:    "different target PC in signed data",
			signer:  parentA,
			change:  func(m *message) { m.PCID = "pc-b" },
			wantErr: true,
		},
		{
			name:      "encrypted for a different PC",
			signer:    parentA,
			recipient: otherPCPublic,
			wantErr:   true,
		},
		{
			name:    "different child account",
			signer:  parentA,
			change:  func(m *message) { m.ChildID = "child-b" },
			wantErr: true,
		},
		{name: "unregistered parent", signer: unknownParent, wantErr: true},
		{name: "unsigned message", wantErr: true},
		{name: "tampered message", signer: parentA, tamper: true, wantErr: true},
		{
			name:    "expired before verification",
			signer:  parentA,
			change:  func(m *message) { m.ExpiresAt = at.Add(-time.Second) },
			wantErr: true,
		},
		{
			name:    "exactly at expiry",
			signer:  parentA,
			change:  func(m *message) { m.ExpiresAt = at },
			wantErr: true,
		},
		{
			name:    "unknown format",
			signer:  parentA,
			change:  func(m *message) { m.Format = "go-screen-time/bonus/v2" },
			wantErr: true,
		},
		{
			name:    "missing grant ID",
			signer:  parentA,
			change:  func(m *message) { m.GrantID = "" },
			wantErr: true,
		},
		{
			name:    "zero minutes",
			signer:  parentA,
			change:  func(m *message) { m.Minutes = 0 },
			wantErr: true,
		},
		{
			name:    "negative minutes",
			signer:  parentA,
			change:  func(m *message) { m.Minutes = -1 },
			wantErr: true,
		},
		{
			name:    "duration overflow",
			signer:  parentA,
			change:  func(m *message) { m.Minutes = 1 << 62 },
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := valid
			if tt.change != nil {
				tt.change(&payload)
			}
			data, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("encode test message: %v", err)
			}

			recipient := tt.recipient
			if recipient == nil {
				recipient = pcPublic
			}
			armored := testArmoredMessage(t, pgp, recipient, tt.signer, data)
			if tt.tamper {
				armored = tamperArmoredMessage(t, armored)
			}

			got, err := VerifyBonusMessage(armored, pcPrivate, trusted, "pc-a", "child-a", at)
			if tt.wantErr {
				if err == nil {
					t.Fatal("message was accepted, want an error")
				}
				if got != (usage.VerifiedBonusGrant{}) {
					t.Errorf("rejected message returned a grant: %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("verify message: %v", err)
			}

			want := usage.VerifiedBonusGrant{
				ID:       grantID,
				ChildID:  "child-a",
				AppID:    payload.AppID,
				Duration: 17 * time.Minute,
			}
			if got != want {
				t.Errorf("grant = %+v, want %+v", got, want)
			}
		})
	}
}

func testKeyPair(t *testing.T, pgp *crypto.PGPHandle, name string) (*crypto.Key, *crypto.Key) {
	t.Helper()
	private, err := pgp.KeyGeneration().AddUserId(name, name+"@example.com").New().GenerateKey()
	if err != nil {
		t.Fatalf("generate test key: %v", err)
	}
	public, err := private.ToPublic()
	if err != nil {
		t.Fatalf("get public test key: %v", err)
	}
	return private, public
}

func testArmoredMessage(
	t *testing.T,
	pgp *crypto.PGPHandle,
	recipient, signer *crypto.Key,
	data []byte,
) string {
	t.Helper()
	builder := pgp.Encryption().Recipient(recipient)
	if signer != nil {
		builder.SigningKey(signer)
	}
	handle, err := builder.New()
	if err != nil {
		t.Fatalf("create test encryptor: %v", err)
	}
	message, err := handle.Encrypt(data)
	if err != nil {
		t.Fatalf("encrypt test message: %v", err)
	}
	armored, err := message.ArmorBytes()
	if err != nil {
		t.Fatalf("armor test message: %v", err)
	}
	return string(armored)
}

func tamperArmoredMessage(t *testing.T, armored string) string {
	t.Helper()
	lines := strings.Split(armored, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "-") || strings.HasPrefix(line, "=") {
			continue
		}
		pos := len(line) / 2
		replacement := byte('A')
		if line[pos] == replacement {
			replacement = 'B'
		}
		lines[i] = line[:pos] + string(replacement) + line[pos+1:]
		return strings.Join(lines, "\n")
	}
	t.Fatal("no armored message data to modify")
	return ""
}
