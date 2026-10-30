// Package manifest describes a release as a list of files and chunks, signed with Ed25519.
// The edge node refuses to apply anything whose manifest does not verify against a pinned key.
// For cosign-signed OCI images, call Cosign() to shell out to `cosign verify` as well.
package manifest

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os/exec"

	"github.com/TYFSADIK/ddil-proof-devops/delta"
)

type File struct {
	Path   string        `json:"path"`
	SHA256 string        `json:"sha256"`
	Mode   uint32        `json:"mode"`
	Chunks []delta.Chunk `json:"chunks"`
}

type Manifest struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Previous  string `json:"previous,omitempty"` // version this is a delta against
	Files     []File `json:"files"`
	Signature string `json:"signature,omitempty"`
}

func (m Manifest) digest() []byte {
	c := m
	c.Signature = ""
	b, _ := json.Marshal(c)
	s := sha256.Sum256(b)
	return s[:]
}

func (m *Manifest) Sign(priv ed25519.PrivateKey) {
	m.Signature = hex.EncodeToString(ed25519.Sign(priv, m.digest()))
}

var ErrUnsigned = errors.New("manifest signature invalid")

func (m Manifest) Verify(pub ed25519.PublicKey) error {
	sig, err := hex.DecodeString(m.Signature)
	if err != nil || !ed25519.Verify(pub, m.digest(), sig) {
		return ErrUnsigned
	}
	return nil
}

// Cosign verifies an OCI image signature with a pinned public key, offline (no Rekor/Fulcio).
func Cosign(image, pubKeyPath string) error {
	cmd := exec.Command("cosign", "verify", "--key", pubKeyPath, "--insecure-ignore-tlog=true", image)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return errors.New("cosign verify failed: " + string(out))
	}
	return nil
}
