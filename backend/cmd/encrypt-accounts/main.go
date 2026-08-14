// Command encrypt-accounts is the seller-side tool that encrypts ChatGPT /
// OpenAI OAuth account records to a customer instance's import public key.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/accountimport"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "gen-keypair":
		if err := runGenKeypair(os.Args[2:]); err != nil {
			fatal(err)
		}
	case "encrypt":
		if err := runEncrypt(os.Args[2:]); err != nil {
			fatal(err)
		}
	case "inspect":
		if err := runInspect(os.Args[2:]); err != nil {
			fatal(err)
		}
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `encrypt-accounts — seller-side ChatGPT account export

Commands:
  gen-keypair              Generate an X25519 import keypair
  encrypt                  Encrypt plaintext account records to a public key
  inspect                  Show bundle metadata (no secrets)

Examples:
  encrypt-accounts gen-keypair
  encrypt-accounts encrypt --public-key "$PUB" --in accounts.json --out bundle.json
  encrypt-accounts inspect --in bundle.json
`)
}

func runGenKeypair(args []string) error {
	fs := flag.NewFlagSet("gen-keypair", flag.ExitOnError)
	outPriv := fs.String("out-private", "", "optional file for the private key (raw-url base64)")
	outPub := fs.String("out-public", "", "optional file for the public key (raw-url base64)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	priv, pub, err := accountimport.GenerateKeyPair()
	if err != nil {
		return err
	}
	if *outPriv != "" {
		if err := os.WriteFile(*outPriv, []byte(priv.Encode()+"\n"), 0o600); err != nil {
			return err
		}
	}
	if *outPub != "" {
		if err := os.WriteFile(*outPub, []byte(pub.Encode()+"\n"), 0o644); err != nil {
			return err
		}
	}
	fmt.Printf("ACCOUNT_IMPORT_PRIVATE_KEY=%s\n", priv.Encode())
	fmt.Printf("public_key=%s\n", pub.Encode())
	fmt.Printf("kid=%s\n", pub.KID())
	fmt.Printf("alg=%s\n", accountimport.Algorithm)
	return nil
}

func runEncrypt(args []string) error {
	fs := flag.NewFlagSet("encrypt", flag.ExitOnError)
	pubFlag := fs.String("public-key", "", "recipient public key (raw-url base64) or @file")
	inFlag := fs.String("in", "", "plaintext account JSON (default stdin)")
	outFlag := fs.String("out", "", "encrypted bundle JSON (default stdout)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*pubFlag) == "" {
		return fmt.Errorf("--public-key is required")
	}
	pubRaw, err := readArg(*pubFlag)
	if err != nil {
		return err
	}
	pub, err := accountimport.ParsePublicKey(strings.TrimSpace(string(pubRaw)))
	if err != nil {
		return err
	}
	input, err := readInput(*inFlag)
	if err != nil {
		return err
	}
	records, err := accountimport.NormalizeSellerRecords(string(input))
	if err != nil {
		return err
	}
	bundle, err := accountimport.EncryptBundle(pub, records)
	if err != nil {
		return err
	}
	raw, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return writeOutput(*outFlag, raw)
}

func runInspect(args []string) error {
	fs := flag.NewFlagSet("inspect", flag.ExitOnError)
	inFlag := fs.String("in", "", "encrypted bundle JSON (default stdin)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	input, err := readInput(*inFlag)
	if err != nil {
		return err
	}
	items, err := accountimport.ParseImportDocuments(string(input))
	if err != nil {
		return err
	}
	var header accountimport.Bundle
	_ = json.Unmarshal(input, &header)
	fmt.Printf("kind=%s\n", valueOr(header.Kind, accountimport.Kind))
	fmt.Printf("version=%d\n", header.Version)
	fmt.Printf("alg=%s\n", valueOr(header.Alg, accountimport.Algorithm))
	fmt.Printf("recipient_kid=%s\n", header.RecipientKID)
	fmt.Printf("items=%d\n", len(items))
	for i, item := range items {
		fmt.Printf("item[%d].id=%s\n", i+1, item.ID)
	}
	return nil
}

func readArg(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "@") {
		return os.ReadFile(strings.TrimPrefix(value, "@"))
	}
	if strings.HasPrefix(value, "/") || strings.HasSuffix(value, ".pub") || strings.HasSuffix(value, ".key") {
		if _, err := os.Stat(value); err == nil {
			return os.ReadFile(value)
		}
	}
	return []byte(value), nil
}

func readInput(path string) ([]byte, error) {
	if strings.TrimSpace(path) == "" || path == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(path)
}

func writeOutput(path string, data []byte) error {
	if strings.TrimSpace(path) == "" || path == "-" {
		_, err := os.Stdout.Write(data)
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func valueOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "encrypt-accounts: %v\n", err)
	os.Exit(1)
}
