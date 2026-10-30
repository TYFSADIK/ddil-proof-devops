// ddil: build and apply chunked, signed updates.
//
//	ddil keygen -key keys/release.key
//	ddil manifest -key keys/release.key -name agent -version 1.3.0 -o release.json FILE...
//	ddil plan -have old-release.json -new release.json      (bytes that must cross the link)
//	ddil verify -pub keys/release.pub release.json
package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/TYFSADIK/ddil-proof-devops/delta"
	"github.com/TYFSADIK/ddil-proof-devops/manifest"
)

func die(f string, a ...any) { fmt.Fprintf(os.Stderr, f+"\n", a...); os.Exit(1) }

func hexfile(p string) []byte {
	b, err := os.ReadFile(p)
	if err != nil {
		die("%v", err)
	}
	k, err := hex.DecodeString(strings.TrimSpace(string(b)))
	if err != nil {
		die("%v", err)
	}
	return k
}

func load(p string) manifest.Manifest {
	var m manifest.Manifest
	b, err := os.ReadFile(p)
	if err != nil || json.Unmarshal(b, &m) != nil {
		die("bad manifest %s", p)
	}
	return m
}

func main() {
	if len(os.Args) < 2 {
		die("usage: ddil keygen|manifest|plan|verify")
	}
	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	key := fs.String("key", "keys/release.key", "")
	pub := fs.String("pub", "keys/release.pub", "")
	name := fs.String("name", "", "")
	ver := fs.String("version", "", "")
	out := fs.String("o", "release.json", "")
	have := fs.String("have", "", "")
	newm := fs.String("new", "", "")
	fs.Parse(os.Args[2:])
	switch os.Args[1] {
	case "keygen":
		p, k, _ := ed25519.GenerateKey(nil)
		os.MkdirAll(filepath.Dir(*key), 0o700)
		os.WriteFile(*key, []byte(hex.EncodeToString(k)), 0o600)
		os.WriteFile(strings.TrimSuffix(*key, ".key")+".pub", []byte(hex.EncodeToString(p)), 0o644)
	case "manifest":
		m := manifest.Manifest{Name: *name, Version: *ver}
		for _, f := range fs.Args() {
			b, err := os.ReadFile(f)
			if err != nil {
				die("%v", err)
			}
			st, _ := os.Stat(f)
			s := sha256.Sum256(b)
			m.Files = append(m.Files, manifest.File{Path: f, SHA256: hex.EncodeToString(s[:]),
				Mode: uint32(st.Mode().Perm()), Chunks: delta.Split(b, delta.Default)})
		}
		m.Sign(ed25519.PrivateKey(hexfile(*key)))
		b, _ := json.MarshalIndent(m, "", " ")
		os.WriteFile(*out, b, 0o644)
		fmt.Println("wrote", *out)
	case "plan":
		old, nw := load(*have), load(*newm)
		h := map[string]bool{}
		total := 0
		for _, f := range old.Files {
			for _, c := range f.Chunks {
				h[c.Hash] = true
			}
		}
		var all []delta.Chunk
		for _, f := range nw.Files {
			all = append(all, f.Chunks...)
			total += func() int {
				n := 0
				for _, c := range f.Chunks {
					n += c.Size
				}
				return n
			}()
		}
		need, n := delta.Plan(all, h)
		fmt.Printf("full release %d bytes, delta %d bytes in %d chunks (%.1f%%)\n", total, n, len(need), 100*float64(n)/float64(max(total, 1)))
	case "verify":
		for _, p := range fs.Args() {
			if err := load(p).Verify(ed25519.PublicKey(hexfile(*pub))); err != nil {
				die("%s: %v", p, err)
			}
			fmt.Println("OK", p)
		}
	default:
		die("unknown command")
	}
}
