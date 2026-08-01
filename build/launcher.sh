#!/bin/bash
# macOS runs this when the .app is opened. The bundle itself may live somewhere
# read-only (e.g. /Applications) and Finder launches it with the working
# directory set to "/", so point the server at a writable database under
# Application Support before handing off. exec keeps the server on this PID, so
# quitting the app (Cmd-Q / Dock → Quit) sends it SIGTERM for a clean shutdown.
set -e
dir="$(cd "$(dirname "$0")" && pwd)"

support="$HOME/Library/Application Support/Backgammon and Wax"
mkdir -p "$support"
export DB_PATH="$support/backgammon.db"

exec "$dir/bgwax-server"
