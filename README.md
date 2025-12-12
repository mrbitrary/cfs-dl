# Cloudflare Stream Downloader

A simple, fast, and robust CLI tool to download videos from [Cloudflare Stream](https://www.cloudflare.com/products/cloudflare-stream/).

## Features

- **Smart Manifest Detection**: Extracts the DASH manifest automatically from an iframe URL.
- **Resolution Selection**: Selects your desired resolution (e.g., `1080p`) or falls back to the closest available one.
- **Parallel Downloading**: Downloads video segments concurrently for maximum seed.
- **Auto-Merge**: Merges audio and video streams into a single MP4 file using `ffmpeg`.
- **Smart Filenames**: Uses the video title from the manifest as the filename.
- **Graceful Shutdown**: safe cancellation with `Ctrl+C`.

## Prerequisites

- **Go**: 1.20+
- **FFmpeg**: Required for merging video and audio streams.

## Installation

```bash
# Clone the repository
git clone https://github.com/mrbitrary/cfs-dl.git
cd cfs-dl

# Install dependencies
go mod tidy

# Build
make build
```

## Usage

```bash
./bin/cfs-dl --check-dependencies
./bin/cfs-dl --url "<IFRAME_URL>" [flags]
```

## Development

```bash
# Run tests
make test

# Check coverage
make coverage

# Clean build artifacts
make clean
```

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--url` | **Required** | N/A | The Cloudflare Stream iframe URL. |
| `--resolution` | Optional | `1080p` | Target video resolution. Falls back to closest available if not found. |
| `--output-dir` | Optional | `data/download` | Directory to save the output file. |
| `--filename` | Optional | `output.mp4` | Output filename. Defaults to the video title extracted from the manifest if available. |

### Example

```bash
./cfs-dl --url "https://customer-xyz.cloudflarestream.com/VIDEO_ID/iframe" --resolution 720p --output-dir ./videos
```

## Project Structure

- `cmd/cfs-dl/`: Main entry point.
- `internal/downloader/`: Downloader logic.
- `internal/model/`: Data models and manifest parsing.
- `internal/merger/`: FFmpeg integration.

## License

GNU GPLv3
