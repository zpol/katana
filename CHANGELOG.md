# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project uses [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.6.0] — 2026-10-16

First tagged release of the public tree (admission control, UI, fleet, and demos), plus multi-scanner and Helm packaging work.

### Added

- Amazon ECR Inspector scanner alongside JFrog Xray, routed by registry host
- Settings → Integrations API/UI for scanners and registry allowlist sync
- Helm chart (`deploy/charts/katana`) for admission controller + webhook TLS
- Local k3d Helm smoke script (`deploy/scripts/demo-k3d-helm.sh`)
- Fleet console cluster honeycomb map
- Docs for multi-scanner setup (`docs/SCANNERS.md`) and disposable AWS E2E helpers

### Changed

- Packaged version defaults (`VERSION`, Docker `KATANA_VERSION`, Helm `appVersion` / chart `version`) set to `1.6.0`

[1.6.0]: https://github.com/zpol/katana/releases/tag/v1.6.0
