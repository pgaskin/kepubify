#!/bin/bash
cd "$(dirname "$0")"
find . -name '*.xhtml' | while read f; do
    tput reset
    echo "$f"
    cat $f | go run . || { echo "Press enter to continue"; read tmp; }
    tput reset
done