// Command e2e-interop is the Go half of the end-to-end encryption interop
// check. It exists so the Go and browser implementations of the format in
// client/e2e.go can be proven byte-compatible against each other rather than
// each only against itself — a format with two implementations is a format
// with two chances to diverge.
//
//	e2e-interop encrypt <key-base64url> <name> <in> <out>
//	e2e-interop decrypt <key-base64url> <in> <out>   # prints the metadata JSON
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/sooua/send.to/client"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 1 {
		return errors.New("usage: e2e-interop encrypt|decrypt|genkey")
	}

	switch args[0] {
	case "encrypt":
		if len(args) != 5 {
			return errors.New("usage: e2e-interop encrypt <key> <name> <in> <out>")
		}
		return encrypt(args[1], args[2], args[3], args[4])

	case "decrypt":
		if len(args) != 4 {
			return errors.New("usage: e2e-interop decrypt <key> <in> <out>")
		}
		return decrypt(args[1], args[2], args[3])

	case "genkey":
		key, err := client.NewE2EKey()
		if err != nil {
			return err
		}
		fmt.Println(key.String())
		return nil

	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func encrypt(keyStr, name, in, out string) error {
	key, err := client.ParseE2EKey(keyStr)
	if err != nil {
		return err
	}

	src, err := os.Open(in)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	reader, err := client.E2EEncrypt(src, key, client.E2EMetadata{Name: name, Type: "application/octet-stream"})
	if err != nil {
		return err
	}

	dst, err := os.Create(out)
	if err != nil {
		return err
	}
	defer func() { _ = dst.Close() }()

	_, err = io.Copy(dst, reader)
	return err
}

func decrypt(keyStr, in, out string) error {
	key, err := client.ParseE2EKey(keyStr)
	if err != nil {
		return err
	}

	src, err := os.Open(in)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	meta, reader, err := client.E2EDecrypt(src, key)
	if err != nil {
		return err
	}

	dst, err := os.Create(out)
	if err != nil {
		return err
	}
	defer func() { _ = dst.Close() }()

	if _, err := io.Copy(dst, reader); err != nil {
		return err
	}

	return json.NewEncoder(os.Stdout).Encode(meta)
}
