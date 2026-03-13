# 🤝 Contributing to The Hive

We welcome contributions that align with our core engineering principles and product vision.

---

## 🏛️ Inviolable Principles

### 1. Zero Technical Debt
Every line of code must be justified, efficient, and well-documented. We prioritize readability and long-term maintainability over clever shortcuts.

### 2. Standard Library Only
To maintain maximum portability, security, and minimal footprint, The Hive uses **100% Go Standard Library** and **Vanilla Javascript**.
- **No external frameworks** (No Gin, No Echo, No React).
- **No external databases** (No SQLite, No Redis).
- **No CGO**.

### 3. Verification & Security
- All new features must include appropriate cryptographic verification.
- Data sanitization is non-negotiable before any information leaves the node.

---

## 🛠️ Development Workflow

### Requirements
- **Go 1.25+**

### Testing
We maintain a strict zero-tolerance policy for race conditions. All PRs must pass:

```bash
# Run all tests with the race detector
go test -race ./...
```

### GitHub Pull Requests
Community Edition validates pull requests with GitHub Actions using the same project entrypoints defined in the `Makefile`:

```bash
make test
make vet
```

To actually block merges when a check fails, configure the repository branch protection or ruleset in GitHub and mark the PR checks as **required**.

For network simulations, use the provided swarm script:
```bash
./scripts/swarm.sh
```

---

## 📜 License

The Hive is released under the **MIT License**. By contributing, you agree to license your work under its terms.

---

[Back to README](README.md)
