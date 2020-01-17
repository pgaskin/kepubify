#!/bin/bash
cd "$(dirname "$0")"

shopt -s globstar
for f in **/*.xhtml; do
    tput reset
    echo "$f"
    cat "$f" | go run . || { echo "Press enter to continue"; read tmp; }
    tput reset
done