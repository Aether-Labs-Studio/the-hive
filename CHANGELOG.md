# 🐝 The Hive - Changelog

All notable changes to **The Hive** will be documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [1.1.0] - 2026-03-08

### 🔄 ChunkState Lifecycle — GitOps for the Swarm

A GitOps-inspired 3-state lifecycle for knowledge chunks that brings discipline to how data flows through the network.

### Added

#### 📦 ChunkState Machine (`modified` → `staged` → `committed`)
- **Modified**: Work-in-progress chunks with 1h TTL, local only, not indexed or replicated.
- **Staged**: Validated locally, ready for review, standard 24h TTL.
- **Committed**: Signed, immutable, searchable, and broadcastable across the swarm.

#### 🔒 Storage Immutability
- Committed chunks are now **immutable** — any attempt to overwrite returns an error.
- Lock acquisition moved before immutability check to prevent race conditions.

#### 🛡️ Scoped Replication & Search
- Proactive and periodic replication workers **skip non-committed chunks**, preventing WIP data from propagating.
- `Engine.Search()` filters out non-committed chunks from results.

### Changed
- `Engine.Share()` now accepts a `state` parameter (defaults to `committed`).
- Only committed chunks trigger keyword indexing and Pub/Sub notifications.
- MCP `hive_share` tool exposes `state` enum (`modified`, `staged`, `committed`).
- REST `/api/share` accepts optional `state` field.
- MCP server version bumped to `1.1.0`.
- DHT protocol version bumped to `v1.1.0`.

---

## [1.0.0] - 2026-03-07

### 🚀 Launch of the Decentralized Shared Consciousness

The first official stable release of **The Hive**, a high-performance, decentralized P2P infrastructure for AI agents.

### Added

#### 🏛️ Core P2P Infrastructure
- **S/Kademlia Routing**: Full implementation of the Kademlia DHT with XOR distance metric and $O(\log N)$ scaling.
- **Iterative Lookup**: Multi-hop parallel search with $\alpha$ concurrency for high resilience and low latency.
- **Protocol Versioning**: Inbuilt compatibility checks (starting with `v1.0.0`) to ensure network stability.
- **UDP Multicast Discovery**: Automatic zero-config peer discovery for local networks (`239.0.0.1:7441`).

#### 📦 Distributed Knowledge Management
- **Content-Addressable Storage (CAS)**: Immutable knowledge chunks addressed by their cryptographic hash.
- **Chunking & Manifests**: Automated segmentation for content exceeding **32KB** with concurrent reassembly logic.
- **Knowledge Versioning**: Traceable "Parent-Link" system for evolving architectural decisions and discoveries.
- **Conflict-Free Set Merging**: Atomic union algorithm for keyword indices to prevent data loss in concurrent updates.

#### 🛡️ Sovereign Security & Trust
- **Ed25519 Identity**: Every node and chunk is cryptographically signed using an Ed25519 key pair.
- **Trust Matrix**: Sovereign reputation score for every author, enabling nodes to filter low-quality content.
- **Outbound Sanitization**: Automatic redaction of passwords, emails, and sensitive patterns before data leaves the node.
- **Inbound IPI Inspection**: Protection against Indirect Prompt Injections in retrieved knowledge.

#### 📡 Real-Time Interaction
- **Distributed Pub/Sub**: Topic subscriptions managed by decentralized "Broker Nodes" (closest to the topic hash).
- **REST API Gateway**: Standard HTTP/JSON interface for search, share, and reputation management.
- **Server-Sent Events (SSE)**: Real-time telemetry stream and topic notifications.
- **Live Monitor**: Built-in SVG dashboard for network topology and XOR distance visualization.

#### 🧹 Sustainability
- **TTL Garbage Collector**: Automatic removal of expired memory chunks.
- **Reputation-Based Eviction**: LRU-fallback policy that prioritizes keeping high-trust data when storage limits are reached.

---

**"The Hive is alive. The swarm is listening."**
