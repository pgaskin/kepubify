#!/bin/bash

set -e

cd "$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

command -v github-release >/dev/null 2>&1 || { echo >&2 "Please install github-release."; exit 1; }

if [[ -z "$GITHUB_TOKEN" ]]; then
    if [[ "$SKIP_UPLOAD" != "true" ]]; then
        echo "Github token not set"
        exit 1
    fi
fi

rm -rf build
mkdir -p build

if [[ -z "$(git describe --abbrev=0 --tags 2>/dev/null)" ]]; then
    echo "No tags found"
    export NO_TAGS=true
    export APP_VERSION=v0.0.1
else
    export NO_TAGS=false
    export APP_VERSION="$(git name-rev --name-only --tags HEAD | sed "s/undefined/$(git describe --abbrev=0 --tags)+$(git rev-parse --short HEAD)-dev/")"
fi

echo "APP_VERSION: $APP_VERSION"

echo "## Changelog" | tee -a build/release-notes.md
if [[ "$NO_TAGS" == "true" ]]; then
    echo "$(git log --oneline)" | tee -a build/release-notes.md
else
    echo "$(git log $(git describe --tags --abbrev=0 HEAD^)..HEAD --oneline)" | tee -a build/release-notes.md
fi

for GOOS in linux windows darwin; do
    for GOARCH in amd64 386; do
        echo "Building kepubify $APP_VERSION for $GOOS $GOARCH"
        GOOS=$GOOS GOARCH=$GOOARCH go build -ldflags "-X main.version=$APP_VERSION" -o "build/kepubify-$GOOS-$(echo $GOARCH|sed 's/386/32bit/g'|sed 's/amd64/64bit/g')$(echo $GOOS|sed 's/windows/.exe/g'|sed 's/linux//g'|sed 's/darwin//g')"
    done
done


if [[ "$SKIP_UPLOAD" != "true" ]]; then
    echo "Creating release"
    GITHUB_TOKEN=$GITHUB_TOKEN github-release release \
        --user geek1011 \
        --repo kepubify \
        --tag $APP_VERSION \
        --name "kepubify $APP_VERSION" \
        --description "$(cat build/release-notes.md)"

    for f in build/kepubify-*;do 
        fn="$(basename $f)"
        echo "Uploading $fn"
        GITHUB_TOKEN=$GITHUB_TOKEN github-release upload \
            --user geek1011 \
            --repo kepubify \
            --tag $APP_VERSION \
            --name "$fn" \
            --file "$f" \
            --replace
    done
fi