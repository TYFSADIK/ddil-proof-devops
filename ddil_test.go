package ddil_test

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/TYFSADIK/ddil-proof-devops/delta"
	"github.com/TYFSADIK/ddil-proof-devops/manifest"
	"github.com/TYFSADIK/ddil-proof-devops/outbox"
)

func TestOutboxSurvivesLinkDrops(t *testing.T) {
	ob, _ := outbox.Open(t.TempDir())
	for i := 0; i < 20; i++ {
		ob.Put("telemetry", outbox.Normal, time.Hour, []byte{byte(i)})
	}
	ob.Put("alert", outbox.Critical, time.Hour, []byte("ALERT"))
	got := map[string]bool{}
	first := ""
	r := rand.New(rand.NewPCG(1, 2))
	flaky := func(m outbox.Message) error {
		if r.Float64() < 0.4 {
			return outbox.ErrLinkDown
		}
		if first == "" {
			first = m.Topic
		}
		got[m.ID] = true
		return nil
	}
	for tries := 0; ob.Len() > 0 && tries < 500; tries++ {
		ob.Drain(flaky, 0)
	}
	if ob.Len() != 0 || len(got) != 21 {
		t.Fatalf("left=%d delivered=%d", ob.Len(), len(got))
	}
	if first != "alert" {
		t.Fatalf("critical lane should drain first, got %s", first)
	}
}

func TestOutboxBudgetAndExpiry(t *testing.T) {
	ob, _ := outbox.Open(t.TempDir())
	ob.Put("old", outbox.Bulk, -time.Second, []byte("x"))
	for i := 0; i < 5; i++ {
		ob.Put("log", outbox.Bulk, time.Hour, make([]byte, 100))
	}
	n, _ := ob.Drain(func(outbox.Message) error { return nil }, 250)
	if n != 2 || ob.Expired != 1 {
		t.Fatalf("sent=%d expired=%d", n, ob.Expired)
	}
	if _, err := ob.Drain(func(outbox.Message) error { return errors.New("x") }, 0); err == nil {
		t.Fatal("expected error")
	}
}

func TestDeltaInsertionIsCheap(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	v1 := make([]byte, 2<<20)
	for i := range v1 {
		v1[i] = byte(r.UintN(256))
	}
	v2 := append(append(append([]byte{}, v1[:1<<20]...), []byte("PATCHED CONFIG LINE\n")...), v1[1<<20:]...)
	c1, c2 := delta.Split(v1, delta.Default), delta.Split(v2, delta.Default)
	have := map[string]bool{}
	store := map[string][]byte{}
	for _, c := range c1 {
		have[c.Hash] = true
		store[c.Hash] = v1[c.Offset : c.Offset+c.Size]
	}
	need, n := delta.Plan(c2, have)
	if n > 3*delta.Default.Max {
		t.Fatalf("delta too large: %d bytes in %d chunks", n, len(need))
	}
	for _, c := range need {
		store[c.Hash] = v2[c.Offset : c.Offset+c.Size]
	}
	out, err := delta.Assemble(c2, store)
	if err != nil || !bytes.Equal(out, v2) {
		t.Fatal("reassembly failed", err)
	}
	t.Logf("2 MiB file, 20-byte insert: shipped %d bytes (%d chunks)", n, len(need))
}

func TestManifestSignature(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	m := manifest.Manifest{Name: "edge-agent", Version: "1.2.0", Files: []manifest.File{{Path: "bin/agent", SHA256: "ab"}}}
	m.Sign(priv)
	if m.Verify(pub) != nil {
		t.Fatal("valid manifest rejected")
	}
	m.Files[0].SHA256 = "cd"
	if m.Verify(pub) == nil {
		t.Fatal("tampered manifest accepted")
	}
}
