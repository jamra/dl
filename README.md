# dl

A robust HTTP file downloader with resume capability, retry logic, and checksum verification.

## Features

- ✅ **Resume downloads** - Continue interrupted downloads with HTTP Range requests
- ✅ **Automatic retry** - Configurable retry attempts with exponential backoff
- ✅ **Checksum verification** - Verify downloads with MD5, SHA256, or SHA512
- ✅ **Progress tracking** - Visual progress bar with speed and ETA
- ✅ **Smart error handling** - HTTP status validation and clear error messages
- ✅ **Timeout configuration** - Prevent hanging on slow servers
- ✅ **Quiet mode** - Silent operation for scripts and automation

## Installation

```bash
go install github.com/jamra/dl
```

## Usage

```bash
dl -url "http://url" [options]
```

### Options

| Flag | Description | Default |
|------|-------------|---------|
| `-url` | URL to download (required) | - |
| `-o` | Output file path | Auto-detected from URL |
| `-r` | Resume incomplete download | `false` |
| `-timeout` | Request timeout in seconds | `30` |
| `-retry` | Maximum retry attempts | `3` |
| `-q` | Quiet mode (no progress bar) | `false` |
| `-md5` | Expected MD5 checksum | - |
| `-sha256` | Expected SHA256 checksum | - |
| `-sha512` | Expected SHA512 checksum | - |

### Examples

**Basic download:**
```bash
dl -url "http://example.com/file.zip"
```

**Download with SHA256 verification:**
```bash
dl -url "http://example.com/file.zip" \
   -sha256 "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
```

**Resume an interrupted download:**
```bash
dl -url "http://example.com/large-file.iso" -o file.iso -r
```

**Download with custom timeout and retry:**
```bash
dl -url "http://slow-server.com/file.zip" -timeout 60 -retry 5
```

**Quiet mode for scripts:**
```bash
dl -url "http://example.com/file.zip" -q -o output.zip
```

## How It Works

### Resume Capability
`dl` uses HTTP Range requests to resume partial downloads. If the server doesn't support Range requests, it will automatically restart the download from the beginning.

### Retry Logic
Failed downloads are automatically retried with exponential backoff:
- Attempt 1: Immediate
- Attempt 2: Wait 1 second
- Attempt 3: Wait 2 seconds
- Attempt 4: Wait 4 seconds
- And so on...

Client errors (4xx except 408) are not retried, as they typically indicate invalid requests.

### Checksum Verification
After a successful download, if a checksum flag is provided, `dl` will compute the file's hash and compare it to the expected value. The download fails if checksums don't match.

## Exit Codes

- `0` - Success
- `1` - Download failed or checksum mismatch

## License

MIT 
