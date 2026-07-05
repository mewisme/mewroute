# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Automatic `readme.md` rendering as HTML for directory routes (case-insensitive filename)

## [0.1.0] - 2026-07-05

### Added

- Initial release of **mewroute**, a static file HTTP router
- Per-directory `.routes.yml` configuration with hot reload
- Route types: rewrite, redirect, file, dist (SPA fallback)
- Directory listing, custom headers, wildcard routes
- Path traversal protection and blocked config file access
- Structured JSON logging (`LOG_LEVEL`)
- Multi-stage Alpine Docker image
- GitHub Actions CI and native multi-arch GHCR publishing

[Unreleased]: https://github.com/mewisme/mewroute/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/mewisme/mewroute/releases/tag/v0.1.0
