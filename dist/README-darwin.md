# BirdNET-Go - macOS Installation

## Tarball Contents

- `birdnet-go` - application binary
- `libonnxruntime.dylib` - ONNX Runtime library (required for Perch v2, BattyBirdNET, and geomodel features)
- `libtensorflowlite_c.dylib` - TensorFlow Lite C library (required for BirdNET v2.4)

## Quick Install

```bash
# Extract the tarball
tar xzf birdnet-go-darwin-*.tar.gz

# Install shared libraries
sudo cp libonnxruntime.dylib libtensorflowlite_c.dylib /usr/local/lib/

# Install the binary
sudo cp birdnet-go /usr/local/bin/

# Run BirdNET-Go
birdnet-go serve
```

## Step-by-Step Guide

### 1. Install shared libraries

Copy the bundled `.dylib` files to `/usr/local/lib/`:

```bash
sudo mkdir -p /usr/local/lib
sudo cp libonnxruntime.dylib libtensorflowlite_c.dylib /usr/local/lib/
```

Verify with:

```bash
ls -la /usr/local/lib/lib{onnxruntime,tensorflowlite_c}.dylib
```

macOS does not use `ldconfig`. The dynamic linker searches `/usr/local/lib/`
by default.

### 2. Install the binary

Copy to a directory in your PATH:

```bash
sudo cp birdnet-go /usr/local/bin/
```

Or run directly from the extracted directory:

```bash
./birdnet-go serve
```

### 3. Gatekeeper approval

macOS may block the binary because it is not notarized. To allow it:

```bash
# Remove the quarantine attribute
xattr -d com.apple.quarantine birdnet-go
xattr -d com.apple.quarantine /usr/local/lib/libonnxruntime.dylib
xattr -d com.apple.quarantine /usr/local/lib/libtensorflowlite_c.dylib
```

Or go to System Settings > Privacy & Security and click "Allow Anyway" after
the first blocked launch attempt.

### 4. First run

On first launch, BirdNET-Go will:

- Create `~/.config/birdnet-go/config.yaml` (or use an existing `/etc/birdnet-go/config.yaml`)
- Download the BirdNET v2.4 model (~45 MB)
- Start the web interface on port 8080

Open `http://localhost:8080` in your browser to access the dashboard.

## Alternative: Per-Session Library Path

If you prefer not to copy libraries system-wide:

```bash
export DYLD_LIBRARY_PATH="$(pwd):$DYLD_LIBRARY_PATH"
./birdnet-go serve
```

Note: `DYLD_LIBRARY_PATH` is stripped by macOS System Integrity Protection
when launching system binaries. It works for directly launched user binaries
but will not propagate through `sudo` or launchd.

## Running as a launchd Service

```bash
sudo cp birdnet-go /usr/local/bin/
sudo mkdir -p /var/lib/birdnet-go

sudo tee /Library/LaunchDaemons/dev.birdnet-go.plist > /dev/null <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>dev.birdnet-go</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/birdnet-go</string>
        <string>serve</string>
    </array>
    <key>WorkingDirectory</key>
    <string>/var/lib/birdnet-go</string>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
</dict>
</plist>
EOF

sudo launchctl load /Library/LaunchDaemons/dev.birdnet-go.plist
```

## Troubleshooting

**"dyld: Library not loaded: libonnxruntime.dylib"**

The library is not in a standard search path. Either:

```bash
sudo cp libonnxruntime.dylib /usr/local/lib/
```

Or set the library path for the current session:

```bash
export DYLD_LIBRARY_PATH="$(pwd):$DYLD_LIBRARY_PATH"
```

**"birdnet-go" cannot be opened because it is from an unidentified developer**

Remove the quarantine attribute:

```bash
xattr -d com.apple.quarantine birdnet-go
```

## More Information

- Documentation: https://github.com/FermedePommerieux/birdnet-go/blob/main/doc/wiki/index.md
- GitHub: https://github.com/FermedePommerieux/birdnet-go
- Releases: https://github.com/FermedePommerieux/birdnet-go/releases
- Discussions: https://github.com/FermedePommerieux/birdnet-go/discussions
