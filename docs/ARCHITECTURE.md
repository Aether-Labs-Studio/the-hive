# 🏛️ Distributed Systems Architecture

This document explains the core principles and mathematical foundations of **The Hive**.

---

## 🏗️ Kademlia Topolgy

The Hive's routing and data storage are based on the **Kademlia DHT (Distributed Hash Table)**.

### XOR Distance Metric
Nodes and data are both addressed by 160-bit IDs. The "distance" between two IDs is calculated as their **XOR bitwise sum**. This creates a logical topology where $O(\log N)$ routing is guaranteed.

### Iterative Routing & Alpha Concurrency
Lookup searches are performed in multiple hops across the network. At each step, the node contacts **$\alpha$ (default: 3)** nodes in parallel. This ensures:
- **Resilience**: A single slow or failed node doesn't block the search.
- **Latency Optimization**: The fastest response moves the search forward.
- **Logarithmic Scaling**: The number of hops grows slowly as the network expands.

### Dynamic Handoff
When a new node joins, its neighbors proactively transfer relevant chunks to it. This ensures that data is always stored by the $k$-closest nodes to its hash, maintaining high availability.

---

## 🛡️ S/Kademlia Identity (Ed25519)

Nodes do not use random IDs. A Hive identity is an **Ed25519 key pair**.
- **NodeID**: The SHA-1 hash of the public key.
- **Verification**: Every message in the swarm is signed by the sender's private key. Peer addresses are cryptographically tied to their identities, making the network resilient against impersonation and Sybil attacks.

---

## 📦 Content-Addressable Storage (CAS)

Knowledge in The Hive is immutable. Chunks are addressed by the hash of their content.
- **Verification**: When a node retrieves a chunk, it verifies the content hash against the requested ID. If they don't match, the data is rejected.
- **Authorship Signatures**: Every memory chunk, manifest, and index is digitally signed by its author.

### Chunking & Manifests
Data larger than **32KB** is automatically segmented into smaller chunks.
- **Manifest (`hive_manifest_v1`)**: A signed document containing a list of chunk IDs.
- **Concurrent Reassembly**: Nodes can download chunks from different peers in parallel to reconstruct the original content.

---

## 🧹 Garbage Collector & Eviction

### Automatic Garbage Collection (GC)
Periodically scans and removes expired memory chunks based on their **Time-To-Live (TTL)**.

### Reputation-Based Eviction
When storage limits are reached (e.g., `-max-storage`), the node purges data based on the **Trust Matrix**:
1. Data from authors with negative reputation is evicted first.
2. **LRU (Least Recently Used)** policy is used as a fallback.

---

## 📡 Distributed Pub/Sub & Brokers

The Hive supports real-time topic subscriptions without a central server.
- **Broker Nodes**: The nodes whose IDs are closest to the hash of a keyword act as "Brokers" for that topic.
- **Subscription**: Nodes register their interest with the brokers using Kademlia's iterative routing.
- **Real-Time Alerts**: When new content is shared, the publisher notifies the brokers, which then broadcast the update to all active subscribers.

---

## 📝 Internal Protocols

The Hive uses a structured **JSONL-Gzip** storage format:
- **Data Chunk**: Gzipped sanitized content inside a `SignedEnvelope` (Data, Signature, PublicKey, ExpiresAt).
- **Index (`hive_index_v1`)**: A signed list of "pointers" (chunk IDs) associated with a specific keyword.
- **Conflict Resolution (Set Merging)**: Keyword indices are merged using a conflict-free set union algorithm, preventing data loss during concurrent network updates.

---

## 📊 Observability (Live Monitor)

The Hive includes a built-in dashboard for real-time visualization of the network state.

- **Force-Directed Graph**: Dynamic SVG visualization of node connectivity and XOR distances.
- **Network State**: Real-time control of **Offline**, **Local**, and **Online** modes.
- **Trending Keywords**: Visualization of the most popular topics in the swarm.
- **Health Metrics**: RAM usage and local cache statistics.

---

[Back to README](../README.md)
