// Command sign-release maintains the trust root behind PinNote's
// self-updater (ADR-0003 D5/D6).
//
//	go run ./tools/sign-release keygen
//	go run ./tools/sign-release sign [-key HEX|-key-file F] ARTIFACT...
//	go run ./tools/sign-release verify [-key HEX] ARTIFACT...
//
// keygen prints a new ed25519 pair as hex: the private half goes into the
// PINNOTE_UPDATE_KEY repository secret, the public half into
// update_public_key.go.
//
// sign writes ARTIFACT.sig containing the raw 64-byte signature over the
// sha256 digest of ARTIFACT - not over the file itself. pkg/updater hashes
// the download as it streams and hands exactly those digest bytes to the
// ed25519 verifier (pkg/updater/verify.go), so signing the digest is both
// what the client expects and the cheaper thing to do with a 13 MB artifact.
//
// verify exists so CI can prove the key it just signed with is the key the
// shipped binary was compiled with, which is the one mistake that bricks
// every future update without any visible symptom.
package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

const (
	// signatureSuffix must match signatureSuffix in update_provider.go: the
	// updater looks for the sidecar under exactly this name.
	signatureSuffix = ".sig"

	// defaultKeyEnv is where the release workflow keeps the private key.
	defaultKeyEnv = "PINNOTE_UPDATE_KEY"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("sign-release: ")
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

const usage = `usage:
  sign-release keygen
  sign-release sign   [-key HEX | -key-file FILE] ARTIFACT...
  sign-release verify [-key HEX] ARTIFACT...

Without -key/-key-file the private key is read from ` + defaultKeyEnv + `.`

func run(args []string) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	switch args[0] {
	case "keygen":
		return keygen()
	case "sign":
		return sign(append([]string{}, args[1:]...))
	case "verify":
		return verify(append([]string{}, args[1:]...))
	}
	return fmt.Errorf("unknown command %q\n%s", args[0], usage)
}

func keygen() error {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return fmt.Errorf("generate key: %w", err)
	}
	fmt.Printf("# 私钥：放进 GitHub Secret %s，永远不要提交\n%s\n\n", defaultKeyEnv, hex.EncodeToString(priv))
	fmt.Printf("# 公钥：粘贴到 update_public_key.go 的 updatePublicKeyHex\n%s\n", hex.EncodeToString(pub))
	return nil
}

// keyFlags is shared by sign and verify: the key arrives either inline, from a
// file, or from the environment, which covers CI and a local run without
// either one having to know about the other.
type keyFlags struct {
	hexValue string
	fileName string
	envName  string
}

func (k *keyFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&k.hexValue, "key", "", "hex-encoded key")
	fs.StringVar(&k.fileName, "key-file", "", "file containing the hex-encoded key")
	fs.StringVar(&k.envName, "key-env", defaultKeyEnv, "environment variable holding the key")
}

// read returns the key bytes, wanting exactly size of them. The raw text is
// tokenised rather than parsed because pasting the whole keygen output -
// comments included - into a secret is the normal thing to do, and the two
// halves are told apart by length.
func (k *keyFlags) read(size int) ([]byte, error) {
	text := k.hexValue
	switch {
	case text == "" && k.fileName != "":
		data, err := os.ReadFile(k.fileName)
		if err != nil {
			return nil, fmt.Errorf("read key file: %w", err)
		}
		text = string(data)
	case text == "":
		text = os.Getenv(k.envName)
	}
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("no key: pass -key or -key-file, or set %s", k.envName)
	}
	var found []byte
	for _, field := range strings.Fields(text) {
		if raw, err := hex.DecodeString(field); err == nil && len(raw) == size {
			found = raw
		}
	}
	if found == nil {
		return nil, fmt.Errorf("no %d-byte hex key found", size)
	}
	return found, nil
}

func sign(args []string) error {
	fs := flag.NewFlagSet("sign", flag.ContinueOnError)
	keys := &keyFlags{}
	keys.register(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	paths := fs.Args()
	if len(paths) == 0 {
		return errors.New("sign: at least one artifact is required")
	}
	raw, err := keys.read(ed25519.PrivateKeySize)
	if err != nil {
		return err
	}
	priv := ed25519.PrivateKey(raw)
	for _, path := range paths {
		digest, err := sha256File(path)
		if err != nil {
			return err
		}
		out := path + signatureSuffix
		if err := os.WriteFile(out, ed25519.Sign(priv, digest), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", out, err)
		}
		fmt.Printf("signed %s -> %s\n", path, out)
	}
	return nil
}

func verify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	keys := &keyFlags{}
	keys.register(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	paths := fs.Args()
	if len(paths) == 0 {
		return errors.New("verify: at least one artifact is required")
	}
	raw, err := keys.read(ed25519.PublicKeySize)
	if err != nil {
		return err
	}
	pub := ed25519.PublicKey(raw)
	for _, path := range paths {
		signature, err := os.ReadFile(path + signatureSuffix)
		if err != nil {
			return fmt.Errorf("read sidecar for %s: %w", path, err)
		}
		if len(signature) != ed25519.SignatureSize {
			return fmt.Errorf("%s%s is %d bytes, want %d", path, signatureSuffix, len(signature), ed25519.SignatureSize)
		}
		digest, err := sha256File(path)
		if err != nil {
			return err
		}
		if !ed25519.Verify(pub, digest, signature) {
			return fmt.Errorf("%s: signature does not verify under the given key", path)
		}
		fmt.Printf("verified %s\n", path)
	}
	return nil
}

func sha256File(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return nil, fmt.Errorf("hash %s: %w", path, err)
	}
	return hash.Sum(nil), nil
}
