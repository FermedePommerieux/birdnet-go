# BirdNET-Go - Linux Installation

## Tarball Contents

- `birdnet-go` - application binary
- `libonnxruntime.so` - ONNX Runtime library (required for Perch v2, BattyBirdNET, and geomodel features)
- `libtensorflowlite_c.so` - TensorFlow Lite C library (required for BirdNET v2.4)

## Quick Install

```bash
# Extract the tarball
tar xzf birdnet-go-linux-*.tar.gz

# Install shared libraries system-wide
# Remove the old link first, so the copy does not write into an older versioned file
sudo rm -f /usr/local/lib/libonnxruntime.so
sudo cp libonnxruntime.so libtensorflowlite_c.so /usr/local/lib/
sudo ldconfig

# Install the binary
sudo cp birdnet-go /usr/local/bin/

# Verify the library is found
ldconfig -p | grep onnxruntime

# Run BirdNET-Go
birdnet-go serve
```

## Step-by-Step Guide

### 1. Install shared libraries

Copy the bundled `.so` files to a directory in the system library search path
and update the linker cache:

```bash
# Remove the old link first, so the copy does not write into an older versioned file
sudo rm -f /usr/local/lib/libonnxruntime.so
sudo cp libonnxruntime.so libtensorflowlite_c.so /usr/local/lib/
sudo ldconfig
```

Verify with:

```bash
ldconfig -p | grep -E "onnxruntime|tensorflowlite"
```

You should see entries like:

```
libonnxruntime.so (libc6,x86-64) => /usr/local/lib/libonnxruntime.so
libtensorflowlite_c.so (libc6,x86-64) => /usr/local/lib/libtensorflowlite_c.so
```

### 2. Install the binary

Copy to a directory in your PATH:

```bash
sudo cp birdnet-go /usr/local/bin/
```

Or run directly from the extracted directory:

```bash
./birdnet-go serve
```

### 3. First run

On first launch, BirdNET-Go will:

- Create `~/.config/birdnet-go/config.yaml` (or use an existing `/etc/birdnet-go/config.yaml`)
- Download the BirdNET v2.4 model (~45 MB)
- Start the web interface on port 8080

Open `http://localhost:8080` in your browser to access the dashboard.

## Alternative: Per-User Library Path

If you do not have root access, set `LD_LIBRARY_PATH` instead of copying to
`/usr/local/lib/`:

```bash
export LD_LIBRARY_PATH="$(pwd):$LD_LIBRARY_PATH"
./birdnet-go serve
```

To make this permanent, add the export to your shell profile or create a
wrapper script.

## Running as a systemd Service

```bash
sudo cp birdnet-go /usr/local/bin/

sudo tee /etc/systemd/system/birdnet-go.service > /dev/null <<'EOF'
[Unit]
Description=BirdNET-Go
After=network-online.target sound.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=/var/lib/birdnet-go
ExecStart=/usr/local/bin/birdnet-go serve
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

sudo mkdir -p /var/lib/birdnet-go
sudo systemctl daemon-reload
sudo systemctl enable --now birdnet-go
```

## Troubleshooting

**"error while loading shared libraries: libonnxruntime.so"**

The library is not in the linker search path. Run:

```bash
# Remove the old link first, so the copy does not write into an older versioned file
sudo rm -f /usr/local/lib/libonnxruntime.so
sudo cp libonnxruntime.so /usr/local/lib/
sudo ldconfig
```

**"error while loading shared libraries: libtensorflowlite_c.so"**

Same fix as above, but for the TFLite library:

```bash
sudo cp libtensorflowlite_c.so /usr/local/lib/
sudo ldconfig
```

**ONNX Runtime features not available (Perch v2, bat models, geomodel)**

Check that the ONNX Runtime library is loadable:

```bash
ldconfig -p | grep onnxruntime
```

If no output, the library is not installed. Follow the shared library
installation steps above.

## More Information

- Documentation: https://github.com/FermedePommerieux/birdnet-go/blob/main/doc/wiki/index.md
- GitHub: https://github.com/FermedePommerieux/birdnet-go
- Releases: https://github.com/FermedePommerieux/birdnet-go/releases
- Discussions: https://github.com/FermedePommerieux/birdnet-go/discussions
