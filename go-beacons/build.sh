#!/bin/bash

# Vibe C2 Go Beacon Build Script
# This script compiles the Go beacon for different platforms

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${BLUE}🌊 Vibe C2 Go Beacon Builder${NC}"
echo

# Parse command line arguments
SERVER_URL="http://localhost:8080"
SLEEP_TIME=30
JITTER_PERCENT=20
TARGET_OS="all"

# Parse arguments
while [[ $# -gt 0 ]]; do
  case $1 in
    --server-url)
      SERVER_URL="$2"
      shift 2
      ;;
    --sleep)
      SLEEP_TIME="$2"
      shift 2
      ;;
    --jitter)
      JITTER_PERCENT="$2"
      shift 2
      ;;
    --os)
      TARGET_OS="$2"
      shift 2
      ;;
    --help)
      echo -e "Usage: ./build.sh [options]"
      echo -e "Options:"
      echo -e "  --server-url URL  Team server URL (default: http://localhost:8080)"
      echo -e "  --sleep SECONDS   Sleep time between check-ins in seconds (default: 30)"
      echo -e "  --jitter PERCENT  Jitter percentage for sleep time (0-50, default: 20)"
      echo -e "  --os TARGET       Target OS to build for (windows, linux, darwin, all)"
      echo -e "  --help            Show this help message"
      exit 0
      ;;
    *)
      echo -e "${RED}[!] Unknown option: $1${NC}"
      exit 1
      ;;
  esac
done

# Create output directory if it doesn't exist
mkdir -p "../target/go-beacons"

# Function to build for a specific OS and architecture
build_for() {
  local os=$1
  local arch=$2
  local extension=""
  
  if [ "$os" == "windows" ]; then
    extension=".exe"
  fi
  
  echo -e "${BLUE}[*] Building for ${os}/${arch}...${NC}"
  
  GOOS=$os GOARCH=$arch go build -ldflags="-s -w -X 'main.defaultServerURL=${SERVER_URL}'" -o "../target/go-beacons/vibe_beacon_${os}_${arch}${extension}" .
  
  if [ $? -eq 0 ]; then
    echo -e "${GREEN}[+] Successfully built: ../target/go-beacons/vibe_beacon_${os}_${arch}${extension}${NC}"
    echo -e "${BLUE}[*] Configuration: Server=${SERVER_URL}, Sleep=${SLEEP_TIME}s, Jitter=${JITTER_PERCENT}%${NC}"
  else
    echo -e "${RED}[!] Build failed for ${os}/${arch}${NC}"
    exit 1
  fi
}

# Build for all platforms or specific platform
if [ "$TARGET_OS" == "all" ] || [ "$TARGET_OS" == "windows" ]; then
  build_for "windows" "amd64"
fi

if [ "$TARGET_OS" == "all" ] || [ "$TARGET_OS" == "linux" ]; then
  build_for "linux" "amd64"
fi

if [ "$TARGET_OS" == "all" ] || [ "$TARGET_OS" == "darwin" ]; then
  build_for "darwin" "amd64"
fi

echo -e "${GREEN}[+] All builds completed successfully${NC}"
echo -e "${BLUE}[*] Binaries are located in: ../target/go-beacons/${NC}"
