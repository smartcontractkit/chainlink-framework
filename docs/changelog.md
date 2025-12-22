# Changelog

All notable changes to the Chainlink Framework are documented here.

This project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Initial Docsify documentation site

### Changed

- _No changes yet_

### Deprecated

- _No deprecations yet_

### Removed

- _No removals yet_

### Fixed

- _No fixes yet_

### Security

- _No security updates yet_

---

## Version History

### Module: multinode

#### [0.x.x] - YYYY-MM-DD

**Added**

- MultiNode client for managing multiple RPC connections
- Node selection strategies: HighestHead, RoundRobin, TotalDifficulty, PriorityLevel
- Health monitoring with automatic failover
- SendOnly node support for transaction broadcasting
- Transaction sender for broadcasting to all healthy nodes

**Changed**

- _Initial release_

---

### Module: chains

#### [0.x.x] - YYYY-MM-DD

**Added**

- Transaction Manager (TxManager) with full lifecycle management
- Head Tracker with backfill support
- Broadcaster for transaction submission
- Confirmer for receipt monitoring
- Finalizer for finality tracking
- Reaper for old transaction cleanup
- Resender for stuck transaction handling

**Changed**

- _Initial release_

---

### Module: capabilities

#### [0.x.x] - YYYY-MM-DD

**Added**

- WriteTarget capability for workflow-based transaction submission
- Chain-agnostic TargetStrategy interface
- Beholder integration for telemetry
- Retry logic with configurable backoff

**Changed**

- _Initial release_

---

### Module: metrics

#### [0.x.x] - YYYY-MM-DD

**Added**

- Balance metrics for account monitoring
- TxManager metrics for transaction observability
- MultiNode metrics for RPC health tracking
- LogPoller metrics for event monitoring

**Changed**

- _Initial release_

---

## Migration Guides

### Migrating from chainlink-common

If you're migrating components that were previously in `chainlink-common`:

1. Update import paths:

   ```go
   // Before
   import "github.com/smartcontractkit/chainlink-common/pkg/multinode"

   // After
   import "github.com/smartcontractkit/chainlink-framework/multinode"
   ```

2. Update go.mod:

   ```bash
   go get github.com/smartcontractkit/chainlink-framework/multinode@latest
   ```

3. Check for interface changes in the [Architecture](architecture.md) documentation.

---

## Release Process

Releases follow semantic versioning:

- **Major (X.0.0)**: Breaking API changes
- **Minor (0.X.0)**: New features, backward compatible
- **Patch (0.0.X)**: Bug fixes, backward compatible

Each module is versioned independently. Check the specific module's go.mod for the current version.

---

## Links

- [GitHub Releases](https://github.com/smartcontractkit/chainlink-framework/releases)
- [Module Dependency Graph](https://github.com/smartcontractkit/chainlink-framework/blob/main/go.md)
