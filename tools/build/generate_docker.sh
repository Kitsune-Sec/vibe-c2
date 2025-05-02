#!/bin/bash
# Vibe C2 - Docker Container Generator
# This script packages a beacon into a Docker container

set -e
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
PROJECT_ROOT="$(realpath $SCRIPT_DIR/../..)"
DOCKER_DIR="$PROJECT_ROOT/target/docker"

GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Default values
SERVER_URL="http://localhost:8080"
IMAGE_NAME="vibe-beacon"
IMAGE_TAG="latest"

print_banner() {
    echo -e "${CYAN}"
    echo "🌊  V I B E  C 2  F R A M E W O R K  🌊"
    echo "         Docker Beacon Builder"
    echo -e "${NC}"
}

print_help() {
    echo -e "${BLUE}Usage:${NC}"
    echo -e "  $0 [OPTIONS]"
    echo
    echo -e "${BLUE}Options:${NC}"
    echo -e "  ${YELLOW}--server-url URL${NC}    Team server URL (default: http://localhost:8080)"
    echo -e "  ${YELLOW}--name NAME${NC}         Docker image name (default: vibe-beacon)"
    echo -e "  ${YELLOW}--tag TAG${NC}           Docker image tag (default: latest)"
    echo -e "  ${YELLOW}--help${NC}              Display this help message"
    echo
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        --server-url)
            SERVER_URL="$2"
            shift 2
            ;;
        --name)
            IMAGE_NAME="$2"
            shift 2
            ;;
        --tag)
            IMAGE_TAG="$2"
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

# Create docker directory
mkdir -p "$DOCKER_DIR"

# Create Dockerfile for the beacon
create_dockerfile() {
    echo -e "${BLUE}[*]${NC} Creating Dockerfile..."
    
    cat > "$DOCKER_DIR/Dockerfile" << EOF
FROM rust:slim as builder
WORKDIR /usr/src/vibe-c2
COPY . .
RUN cargo build --bin vibe-beacon --release

FROM debian:buster-slim
RUN apt-get update && apt-get install -y \\
    libssl-dev \\
    ca-certificates \\
    && rm -rf /var/lib/apt/lists/*

WORKDIR /opt/vibe-c2
COPY --from=builder /usr/src/vibe-c2/target/release/vibe-beacon /opt/vibe-c2/

# Create entrypoint script
RUN echo '#!/bin/bash' > /opt/vibe-c2/start.sh && \\
    echo 'echo "🌊 Vibe C2 Beacon (Docker Edition) 🌊"' >> /opt/vibe-c2/start.sh && \\
    echo 'exec /opt/vibe-c2/vibe-beacon --server $SERVER_URL' >> /opt/vibe-c2/start.sh && \\
    chmod +x /opt/vibe-c2/start.sh

ENTRYPOINT ["/opt/vibe-c2/start.sh"]
EOF

    echo -e "${GREEN}[+]${NC} Dockerfile created successfully."
}

# Build the Docker image
build_docker_image() {
    echo -e "${BLUE}[*]${NC} Building Docker image ${YELLOW}${IMAGE_NAME}:${IMAGE_TAG}${NC}..."
    
    # Build the image
    cd "$PROJECT_ROOT"
    docker build -t "${IMAGE_NAME}:${IMAGE_TAG}" -f "$DOCKER_DIR/Dockerfile" .
    
    echo -e "${GREEN}[+]${NC} Docker image built successfully."
    echo -e "${GREEN}[+]${NC} Image: ${IMAGE_NAME}:${IMAGE_TAG}"
}

# Create a run script
create_run_script() {
    echo -e "${BLUE}[*]${NC} Creating Docker run script..."
    
    cat > "$DOCKER_DIR/run_beacon.sh" << EOF
#!/bin/bash
# Vibe C2 - Docker Beacon Runner

# Run the Docker container
docker run -d --name vibe-beacon-instance "${IMAGE_NAME}:${IMAGE_TAG}"

echo "🌊 Vibe C2 Docker Beacon started! 🌊"
echo "Container: vibe-beacon-instance"
echo "To view logs: docker logs vibe-beacon-instance"
echo "To stop: docker stop vibe-beacon-instance"
EOF

    chmod +x "$DOCKER_DIR/run_beacon.sh"
    
    echo -e "${GREEN}[+]${NC} Run script created: $DOCKER_DIR/run_beacon.sh"
}

# Main execution
print_banner
echo -e "${BLUE}[*]${NC} Preparing Docker build environment..."
echo -e "${BLUE}[*]${NC} Server URL: ${YELLOW}${SERVER_URL}${NC}"

# Check if Docker is installed
if ! command -v docker &> /dev/null; then
    echo -e "${RED}[!]${NC} Docker is not installed. Please install Docker to continue."
    exit 1
fi

create_dockerfile
build_docker_image
create_run_script

echo -e "${CYAN}"
echo "🌊  V I B E  C 2  F R A M E W O R K  🌊"
echo "     Docker Beacon Build Complete!"
echo -e "${NC}"
echo -e "To run the beacon container, use:"
echo -e "${YELLOW}  $DOCKER_DIR/run_beacon.sh${NC}"
echo ""
