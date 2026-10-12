#!/bin/bash

# Stops every benchmark server and client still running from output/bin:
# SIGINT first, then SIGKILL for whatever is left.
pattern='output/bin/[a-z_]+\.(server|client)'
if pgrep -f "$pattern" >/dev/null 2>&1; then
    pkill -INT -f "$pattern" 2>/dev/null
    for _ in 1 2 3 4 5 6 7 8 9 10; do
        pgrep -f "$pattern" >/dev/null 2>&1 || break
        sleep 0.5
    done
    pkill -KILL -f "$pattern" 2>/dev/null
fi
true
