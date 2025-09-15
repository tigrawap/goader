# Goader Repository Map

## Overview
Goader is a Go-based benchmarking utility for testing various request engines with configurable load patterns and output formats.

## Core Files
- `goader.go` - Main entry point and CLI handling
- `interfaces.go` - Core interfaces and type definitions
- `requesters.go` - Request engine implementations (disk, meta, null, sleep, http/upload, s3)
- `targets.go` - Target/URL generation and management
- `adjusters.go` - Load adjustment algorithms (RPS, latency-based)
- `emitters.go` - Request emission logic
- `output.go` - Output formatters (human, json)
- `timeline.go` - Timeline generation for request visualization
- `auth.go` - Authentication handling
- `format.go` - Data formatting utilities
- `payload_getters.go` - Payload generation logic

## Platform-Specific
- `meta_unix.go` - Unix filesystem operations
- `meta_windows.go` - Windows filesystem operations

## Subdirectories
### `utils/`
- `math.go` - Mathematical utilities
- `paths.go` - Path manipulation utilities
- `random.go` - Random data generation

### `ops/`
- `xattr.go` - Extended attributes interface
- `xattr_linux.go` - Linux xattr implementation
- `xattr_other.go` - Other platforms xattr implementation

## Key Features
- Multiple request engines: disk, meta, null, sleep, http/upload, s3
- URL templating with XXXXX, NNNN, RRRR patterns
- Load patterns: RPS/WPS, thread-based, max-latency search
- Output formats: human-readable, JSON
- Timeline visualization
- Fair random distribution options

## Build & Config
- `go.mod` - Go module definition
- `build.sh` - Build script
- `dist/` - Distribution binaries