# DDIL-Proof DevOps

Private research repo. CI/CD and telemetry for denied, degraded, intermittent, and limited networks: store-and-forward queues, delta updates, signed artifacts, and k3s edge nodes.

Written in **Go**: standard library only, a static binary per edge node, no runtime needed.

Full write-up: [`docs/project-page.md`](docs/project-page.md)

## What's here

| Path | Purpose |
|---|---|
| `outbox/` | Durable store-and-forward queue: atomic write-then-rename, priority lanes (critical, normal, bulk), TTL expiry with counters, byte budget for metered links, full-jitter backoff |
| `delta/` | **Content-defined chunking** (Gear/FastCDC-style). A 20-byte insert into a 2 MiB file ships about 1 chunk instead of the whole file |
| `manifest/` | Release manifest (files, chunks, hashes) signed with Ed25519, plus an offline `cosign verify` wrapper for OCI images |
| `cmd/ddil` | CLI: `keygen`, `manifest`, `plan` (bytes that must cross the link), `verify` |
| `deploy/k3s-edge-agent.yaml` | Edge DaemonSet that pulls only from a local registry mirror |
| `deploy/tc-degrade.sh` | netem script that emulates a 256 kbit/s, 600 ms, 5% loss link for tests |

## Quickstart

```bash
make test
make build && ./bin/ddil keygen && ./bin/ddil manifest -name agent -version 1.0.0 -o v1.json bin/ddil
```

## Next steps
- [ ] NATS JetStream transport for `outbox.Sender` (`nats.go`, a stream per priority lane, `Nats-Msg-Id` header = `Message.ID` for dedupe)
- [ ] Edge apply step: fetch missing chunks, `Assemble`, verify the manifest, atomic swap, health check, roll back on failure
- [ ] Measure on `tc-degrade.sh`: time to deliver a critical alert behind 10 MB of bulk logs, and update size vs full image
- [ ] Ansible playbook to bootstrap k3s and the registry mirror on the Pis
