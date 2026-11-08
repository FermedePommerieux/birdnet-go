# BirdNET-Go

<p align="center">
  <img src="doc/BirdNET-Go-logo.webp" />
</p>
<p align="center">
  <!-- Project Status -->
  <a href="https://github.com/FermedePommerieux/birdnet-go/releases">
    <img src="https://img.shields.io/github/v/release/FermedePommerieux/birdnet-go?include_prereleases&style=flat-square&color=blue">
  </a>
  <a href="https://creativecommons.org/licenses/by-nc-sa/4.0/">
    <img src="https://badgen.net/badge/License/CC-BY-NC-SA%204.0/green">
  </a>
  <img src="https://badgen.net/badge/OS/Linux%2C%20Windows%2C%20macOS/blue">

  <br>

  <!-- Code Quality -->
  <a href="https://golang.org">
    <img src="https://img.shields.io/badge/Built%20with-Go-teal?style=flat-square&logo=go">
  </a>
  <br>

  <!-- Fork activity -->
  <a href="https://github.com/FermedePommerieux/birdnet-go/issues">
    <img src="https://img.shields.io/github/issues/FermedePommerieux/birdnet-go?style=flat-square&color=red">
  </a>
  <a href="https://discord.gg/gcSCFGUtsd">
    <img src="https://img.shields.io/discord/1416220486433767436?style=flat-square&logo=discord&color=5865F2&label=Discord">
  </a>

</p>

**Realtime soundscape analyser for birds, wildlife, and bats.**

Self-hosted, 24/7, local AI inference. BirdNET-Go ingests soundcard input or network audio streams, runs multi-model classification, and presents detections in a fast web UI. Runs on a Raspberry Pi.

## About this fork

This is the [Ferme de Pommerieux fork](https://github.com/FermedePommerieux/birdnet-go) of [tphakala/BirdNET-Go](https://github.com/tphakala/birdnet-go). We maintain it because our installation uses PostgreSQL for BirdNET-Go's v2 detection store. We use the PostgreSQL path in production, but our deployment is one environment, not a guarantee for every installation. SQLite remains the default; MySQL/MariaDB remains available. PostgreSQL is an explicit third choice in this fork and is not an upstream feature.

We intend to follow upstream releases through automatically proposed updates, reviewed and tested before integration, while maintaining the PostgreSQL-specific code and tests here. This fork has its own [issues](https://github.com/FermedePommerieux/birdnet-go/issues) and [releases](https://github.com/FermedePommerieux/birdnet-go/releases); upstream release badges, images and installation commands do not describe this fork. See the [PostgreSQL guide](doc/wiki/postgresql.md) for tested versions, setup, and migration limits. For general BirdNET-Go features and upstream community support, see the [original project](https://github.com/tphakala/birdnet-go).

## Highlights

- **Multi-model AI gallery**: install BirdNET v2.4, Google Perch v2, BattyBirdNET regional bat classifiers, and the BirdNET Geomodel v3.0 from inside the app, no rebuild needed.
- **Run multiple models in parallel** against separate audio sources and let cross-model agreement boost confidence on shared detections.
- **Live spectrogram streaming** and **detection heatmaps** rendered straight in the browser.
- **Alert rules engine** that routes detections to Discord, Slack, Telegram, ntfy, Pushover, Gotify, Matrix, webhooks, browser push, MQTT (with Home Assistant discovery), shell scripts, and BirdWeather.
- **Production-ready ops**: onboarding wizard, OIDC/SSO, TLS certificate management, hot-reload settings, system health page, database doctor, and one-click support dumps.
- **Installable as a PWA**, with a multilingual UI and species names in 40+ languages.
- **Local-only by default**. Optional Sentry telemetry is strictly opt-in.

## Quick install

For this fork on Debian, Ubuntu, or Raspberry Pi OS, use its installer and a [published fork release](https://github.com/FermedePommerieux/birdnet-go/releases):

```bash
curl -fsSL https://raw.githubusercontent.com/FermedePommerieux/birdnet-go/main/install.sh -o install.sh
bash ./install.sh
```

For containers, use the public fork image `ghcr.io/fermedepommerieux/birdnet-go:latest`, or pin the [published `v2026.11.8` release](https://github.com/FermedePommerieux/birdnet-go/releases/tag/v2026.11.8). The fork installer selects `latest` by default. The [host-network Compose file](Docker/docker-compose.host.yml) and [fork installation guide](doc/wiki/installation.md) describe the deployment layout; verify the image reference before deploying. Fork images target Linux `amd64` and `arm64`. Upstream images do not include this fork's PostgreSQL changes. The Unraid host template is untested on real Unraid.

Choose SQLite for a new local installation unless you need an external database. To use PostgreSQL 17 or 18, provision a UTF8 database and follow the [PostgreSQL configuration guide](doc/wiki/postgresql.md) before starting the fork. Switching the selected backend starts a separate history: the installer and settings screen do not copy SQLite or MySQL/MariaDB detections into PostgreSQL.

## Web Dashboard

<img src="doc/BirdNET-Go-dashboard.webp" />

## Features

### Detection

- **BirdNET v2.4** (default, embedded, 6,500+ bird species)
- **Google Perch v2** via ONNX (14,795 species across birds, insects, amphibians, and mammals) with taxonomy-based species names
- **BattyBirdNET** bat classifiers, 11 regional models covering Africa, the Americas, East Asia, Europe, the Middle East, South Asia, Southeast Asia, and the USA (Linux + ultrasonic-capable device)
- **BirdNET Geomodel v3.0** for location-based range filtering (12,012 species)
- **Cross-model detection consensus**: agreement between models strengthens confidence and flags disagreements for review
- **Custom classifiers**: bring your own TFLite model and label set
- **Configurable false-positive filtering** for accurate results: Deep Detection (repeat-confirmation within a 15-second window), per-species dynamic thresholds, location-based range filter, privacy and dog-bark filters, and per-classifier bat false-positive levels ([guide](doc/wiki/guide.md#deep-detection))
- Per-model and per-source confidence thresholds

### Audio inputs

- Soundcard capture and RTSP / RTSPS streams, including multiple sources in parallel with independent model assignment
- Sample rates up to 256 kHz for ultrasonic bat detection
- Audio liveness watchdog with tiered recovery for flaky streams
- Stream sample-rate probing and per-model recommendation banners in the UI
- Audio equalizer, per-source quiet hours, daylight filter, and extended capture mode
- Offline analysis of audio files

### Interface

- Svelte 5 + TypeScript single-page app
- Installable as a Progressive Web App (PWA)
- Onboarding wizard for first-run setup
- Live spectrogram visualization for active streams ([live audio streaming](doc/wiki/guide.md#live-audio-streaming))
- Detection heatmaps with ONNX-accelerated rendering
- Customizable dashboard layout, color schemes, and a "Currently Hearing" card
- Multiselect and bulk actions on the detections list
- Browser terminal (xterm.js over WebSocket PTY) for in-app administration
- UI languages: English, German, French, Spanish, Portuguese, Dutch, Polish, Italian, Czech, Slovak, Hungarian, Finnish, Swedish, Danish, Norwegian (Bokmål), Latvian
- Species names in 40+ languages

### Alerts and integrations

- Configurable alert rules engine with per-rule conditions, schedules, and delivery targets
- Multi-target delivery via [shoutrrr](https://github.com/nicholas-fedor/shoutrrr): Discord, Slack, Telegram, ntfy, Pushover, Gotify, Matrix, Bark, IFTTT, and more
- Webhooks with custom templates, shell-script hooks, and browser push notifications
- MQTT publishing with Home Assistant auto-discovery
- BirdWeather.com API integration
- Prometheus metrics endpoint
- Live spectrogram and realtime log output for OBS overlays on bird-feeder streams

### Storage and data

- SQLite (default) or MySQL/MariaDB with retry-aware write paths for contention;
  only the tested LTS releases are supported (MySQL 8.4 and 9.7; MariaDB 10.11,
  11.4, 11.8 or 12.3), and MySQL Innovation releases are not supported
- PostgreSQL 17 and 18 support for the v2 datastore in this fork; see the
  [PostgreSQL configuration and testing guide](doc/wiki/postgresql.md)
- Automatic backups with real-time status polling
- Format-aware audio clip export
- Embedded eBird/Clements taxonomy (2,374 genera, 254 families, 11,145 species) for fast offline lookups

### Operations

- System Health diagnostics page covering audio pipeline, models and inference, network, and the datastore
- Database doctor for diagnosis and schema repair
- Help & Support page with guided bug reporting and one-click support dumps
- OIDC / SSO with Google, GitHub, and generic providers, including RP-Initiated Logout
- TLS certificate management UI with transactional writes and backup/restore
- Hot-reload for settings, model installs, and per-source model assignments (no restart)
- Optional, opt-in Sentry telemetry with strict privacy filtering

### Platform

- Linux, Windows, and macOS
- Single static binary with the BirdNET TFLite model embedded
- Optional ONNX Runtime for Perch, BattyBirdNET, and BirdNET Geomodel v3.0 ([install guide](doc/wiki/onnx-runtime-installation.md))
- Multi-arch Docker images
- Runs comfortably on a Raspberry Pi 4 or equivalent 64-bit single-board computer

## Documentation

- [FAQ](doc/wiki/faq.md) - common questions, issues, and workarounds
- [User guide](doc/wiki/guide.md)
- [Installation](doc/wiki/installation.md)
- [Hardware recommendations](doc/wiki/hardware.md)
- [ONNX Runtime installation](doc/wiki/onnx-runtime-installation.md)
- [Detection pipeline](doc/wiki/detection-pipeline.md)
- [Database Doctor](doc/wiki/database-doctor.md)
- [PostgreSQL configuration, limits and testing](doc/wiki/postgresql.md) (fork feature)
- [Training a custom classifier](doc/wiki/training-a-custom-classifier.md)
- [Cloudflare Tunnel](doc/wiki/cloudflare_tunnel_guide.md)
- [Security](doc/wiki/security.md)
- [Telemetry and privacy](doc/wiki/telemetry-privacy.md)
- [RTSP troubleshooting](doc/wiki/rtsp-troubleshooting.md)

## Development setup

> See [CONTRIBUTING.md](CONTRIBUTING.md) for the full guide.

```bash
git clone https://github.com/FermedePommerieux/birdnet-go.git
cd birdnet-go

# Install Task (if not already installed)
# Linux: sh -c "$(curl --location https://taskfile.dev/install.sh)" -- -d -b /usr/local/bin
# macOS: brew install go-task

task setup-dev    # installs Go 1.25, Node LTS, build tools, linters, Playwright
task              # build
task dev_server   # hot-reload dev server (or: air realtime)
```

## Community

Join the [Discord server](https://discord.gg/gcSCFGUtsd) for support, discussions, and updates.

## Related projects

### Core and extensions

- [BirdNET-Analyzer](https://github.com/birdnet-team/BirdNET-Analyzer): upstream BirdNET project
- [BirdNET-Go Classifiers](https://github.com/tphakala/birdnet-go-classifiers): enhanced BirdNET classifiers with additional species
- [BattyBirdNET-Analyzer](https://github.com/rdz-oss/BattyBirdNET-Analyzer): bat classifier models, installable via the in-app gallery

### Companion tools

- [birda](https://github.com/tphakala/birda): fast CLI for offline bird species detection on existing audio files, using BirdNET and Perch AI models
- [birda-gui](https://github.com/tphakala/birda-gui): desktop GUI for birda (Electron + Svelte 5) for audio analysis, detection browsing, and species visualization

### System integration

- [Cockpit BirdNET-Go](https://github.com/tphakala/cockpit-birdnet-go): web-based system management plugin using the Cockpit framework

### Migration tools

- [BirdNET-Pi2Go](https://github.com/tphakala/birdnet-pi2go): database converter for migrating from BirdNET-Pi (deprecated, not compatible with the current BirdNET-Go database schema; a native import is planned)

### Hardware solutions

- [BirdNET-Go ESP32 RTSP Microphone](https://github.com/Sukecz/birdnetgo-esp32-rtsp-mic): ESP32-based RTSP streaming microphone
- [ESP32 Audio Streamer](https://github.com/jpmurray/esp32-audio-streamer): alternative ESP32 RTSP streaming solution
- [M5Stack Atom Echo RTSP Mic](https://github.com/stedrow/birdnetgo-m5stack-atom-echo-rtsp-mic): RTSP audio server for M5Stack Atom Echo, no soldering required
- [M5Stack AtomS3 Lite PDM Mic](https://github.com/matthew73210/birdnetgo-m5stack-AtomS3-Lite-PDM-rtsp-mic): RTSP audio server with MEMS PDM microphone

### Mobile apps

- [Perch](https://github.com/arunrajiah/perch): open-source Android/iOS companion app. Connects to BirdNET-Go via the BirdWeather API. Live detection feed, audio playback, species browser, 14-day chart, and notifications for favourite species. MIT licensed.
- [BirdFeeder](https://github.com/kj7ppk/birdfeeder): open-source Android streaming app. Publishes real-time RTSP audio for consumption as a BirdNET-GO stream.

## Contributing

Contributions are welcome.

For setup, workflow, and quality gates, see [CONTRIBUTING.md](CONTRIBUTING.md):

- [TL;DR quick start](CONTRIBUTING.md#tldr---quick-start-for-experienced-developers): 5-minute setup
- [Development workflow](CONTRIBUTING.md#development-workflow): hot reload, git hooks, testing
- [License and privacy](CONTRIBUTING.md#license-and-legal): CC BY-NC-SA 4.0, privacy by design

All contributions must follow privacy-by-design principles, the automated code-quality gates, and the CC BY-NC-SA 4.0 license terms.

## Data sources

### Taxonomy data

BirdNET-Go embeds taxonomy data derived from the eBird/Clements Checklist:

- **Source**: [eBird API v2](https://api.ebird.org/v2/ref/taxonomy/ebird)
- **Copyright**: (c) Cornell Lab of Ornithology
- **License**: used under eBird API Terms of Use for non-commercial purposes
- **Attribution**: taxonomy data powered by [eBird.org](https://ebird.org)
- **Coverage**: 2,374 genera, 254 families, 11,145 species

For more information, see the [eBird Taxonomy](https://ebird.org/science/use-ebird-data/the-ebird-taxonomy).

## License

Creative Commons Attribution-NonCommercial-ShareAlike 4.0 International.

## Authors and acknowledgements

BirdNET-Go was created and is maintained upstream by Tomi P. Hakala. The PostgreSQL fork is maintained by FermedePommerieux.

A growing list of community contributors keeps the project moving forward. The current list lives on the [GitHub contributors page](https://github.com/tphakala/birdnet-go/graphs/contributors).

The BirdNET AI model is by the K. Lisa Yang Center for Conservation Bioacoustics at the Cornell Lab of Ornithology in collaboration with Chemnitz University of Technology (Stefan Kahl, Connor Wood, Maximilian Eibl, Holger Klinck).

Google Perch v2 ONNX conversion by [Justin Chuby](https://huggingface.co/justinchuby/BirdNET-onnx). BattyBirdNET bat classifier models by [R.D. Zinck](https://github.com/rdz-oss/BattyBirdNET-Analyzer). BirdNET label translations by Patrick Levin for the BirdNET-Pi project by Patrick McGuire.
