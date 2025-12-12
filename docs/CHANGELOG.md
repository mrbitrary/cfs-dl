# Changelog

All notable changes to this project will be documented in this file.

## [0.1.0] - 2025-12

### Added
- **Core Functionality**:
    - Automatic DASH manifest parsing from Cloudflare Stream iframe URLs.
    - Concurrent downloading of video and audio segments.
    - Automatic merging of streams using `ffmpeg`.
- **Smart Features**:
    - Resolution selection with automatic fallback to closest available quality.
    - Filename extraction from manifest metadata.
    - Graceful shutdown on interrupt (Ctrl+C).
- **Safety**:
    - Dependency check (`--check-dependencies`) to verify `ffmpeg` installation.
    - Robust error handling and temporary file cleanup.
- **CLI**:
    - Flags for URL, resolution, output directory, and filename.
    - Clean help output with default values.
