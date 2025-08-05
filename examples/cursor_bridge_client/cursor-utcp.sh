#!/bin/bash
# Cursor UTCP Bridge CLI Wrapper
# Provides simplified commands for common operations

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CLI_BINARY="${SCRIPT_DIR}/cursor-utcp"
BRIDGE_URL="${CURSOR_UTCP_URL:-http://localhost:8080}"

# Build the binary if it doesn't exist
if [ ! -f "$CLI_BINARY" ]; then
    echo "Building cursor-utcp binary..."
    (cd "$SCRIPT_DIR" && GOWORK=off go build -o cursor-utcp main.go)
fi

# Function to execute CLI with common parameters
execute_cli() {
    local cmd_args=("-url" "$BRIDGE_URL")
    "$CLI_BINARY" "${cmd_args[@]}" "$@"
}

# Parse command
case "$1" in
    list)
        execute_cli -cmd list "${@:2}"
        ;;
    search)
        shift
        execute_cli -cmd search -query "$@"
        ;;
    info)
        execute_cli -cmd info -tool "$2" "${@:3}"
        ;;
    call)
        tool="$2"
        shift 2
        # Check if -i flag is provided for input
        if [ "$1" = "-i" ]; then
            execute_cli -cmd call -tool "$tool" -input "$2" "${@:3}"
        else
            # Build JSON from key=value pairs
            json="{"
            first=true
            for arg in "$@"; do
                if [[ "$arg" =~ ^([^=]+)=(.*)$ ]]; then
                    key="${BASH_REMATCH[1]}"
                    value="${BASH_REMATCH[2]}"
                    if [ "$first" = false ]; then
                        json+=","
                    fi
                    json+="\"$key\":\"$value\""
                    first=false
                fi
            done
            json+="}"
            execute_cli -cmd call -tool "$tool" -input "$json"
        fi
        ;;
    health)
        execute_cli -cmd health "${@:2}"
        ;;
    refresh)
        execute_cli -cmd refresh "${@:2}"
        ;;
    help|--help|-h)
        cat << EOF
Cursor UTCP Bridge CLI

Usage:
  cursor-utcp list                    List all available tools
  cursor-utcp search <query>          Search for tools
  cursor-utcp info <tool>             Get information about a tool
  cursor-utcp call <tool> [args]      Call a tool
  cursor-utcp health                  Check bridge server health
  cursor-utcp refresh                 Refresh tool cache

Examples:
  cursor-utcp list
  cursor-utcp search echo
  cursor-utcp info cli.echo
  cursor-utcp call cli.echo message="Hello World"
  cursor-utcp call http.api -i '{"method":"GET","path":"/users"}'

Environment Variables:
  CURSOR_UTCP_URL       Bridge server URL (default: http://localhost:8080)

EOF
        ;;
    *)
        echo "Unknown command: $1"
        echo "Run 'cursor-utcp help' for usage information"
        exit 1
        ;;
esac
