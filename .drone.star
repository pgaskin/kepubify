go = "1.14.7"

pipeline = [{
    "name": pname,
    "kind": "pipeline",
    "type": "docker",
    "steps": [{
        "name": name,
        "image": "golang:%s-buster" % (go),
        "environment": {
            "GO111MODULE": "on",
        },
        "commands": [cmd],
    } for (name, cmd) in psteps],
    "trigger": {
        "event": {
            "exclude": ["promote"],
        },
    },
    "depends_on": pdep,
} for (pname, psteps, pdep) in [
    ("kepub", [
        ("test-html", "go test -mod=readonly -run \"^TestMod_\" golang.org/x/net/html -v"),
        ("test", "go test -mod=readonly ./kepub -v -cover"),
    ], []),
    ("kepubify", [
        ("test", "go test -mod=readonly ./cmd/kepubify -v -cover"),
        ("run", "go run -mod=readonly ./cmd/kepubify --help"),
    ], ["kepub"]),
    ("covergen", [
        ("test", "go test -mod=readonly ./cmd/covergen -v -cover"),
        ("run", "go run -mod=readonly ./cmd/covergen --help"),
    ], []),
    ("seriesmeta", [
        ("test", "go test -mod=readonly ./cmd/seriesmeta -v -cover"),
        ("run", "go run -mod=readonly ./cmd/seriesmeta --help"),
    ], []),
]] + [{
    "name": "release",
    "kind": "pipeline",
    "type": "docker",
    "steps": [{
        "name": "notes",
        "image": "golang:%s-buster" % (go),
        "commands": [
            "mkdir -p build",
            "git fetch --tags",
            "git describe --tags --always | tee build/version",
            "git log \"$(git describe --tags --abbrev=0 HEAD~1)..HEAD\" --oneline --format='%h %s' | tee build/notes.md",
        ]
    }] + [{
        "name": "x-%s-%s" % (app, platform.replace("/", "-")),
        "image": "docker.elastic.co/beats-dev/golang-crossbuild:%s-%s" % (go, img),
        "environment": {
            "GO111MODULE": "on",
        },
        "command": [
            "--platforms", platform,
            "--build-cmd", "go env; CGO_ENABLED=%s go build -ldflags \"-s -w -X main.version=$(cat build/version)\" -o \"build/%s%s\" %s" % (cgo, app, suffix, "./cmd/" + app),
        ],
    } for (img, platform, suffix) in [
        ("main",   "linux/amd64",   "-linux-64bit"),
        ("main",   "linux/386",     "-linux-32bit"),
        ("arm",    "linux/armv6",   "-linux-arm"),
        ("arm",    "linux/arm64",   "-linux-arm64"),
        ("darwin", "darwin/amd64",  "-darwin-64bit"),
        ("main",   "windows/amd64", "-windows-64bit.exe"),
        ("main",   "windows/386",   "-windows-32bit.exe"),
    ] for (app, cgo) in [
        ("kepubify",   "0"),
        ("covergen",   "0"),
        ("seriesmeta", "1"),
    ]] + [{
        "name": "debian",
        "image": "golang:%s-buster" % (go),
        "environment": {
            "GO111MODULE": "on",
        },
        "commands": [
            "go install github.com/goreleaser/nfpm/cmd/nfpm",
            "VERSION=$(cat build/version | tr -d v) nfpm pkg -f nfpm.yaml -t build/kepubify_$(cat build/version | tr -d v)_amd64.deb",
        ],
    }, {
        "name": "ls",
        "image": "golang:%s-buster" % (go),
        "commands": [
            "apt update && apt install -y file",
            "file build/*",
            "ls -lah build/*",
        ],
    }, {
        "name": "release",
        "image": "plugins/github-release",
        "settings": {
            "api_key": {
                "from_secret": "GITHUB_TOKEN",
            },
            "notes": "build/notes.md",
            "draft": True,
            "files": [
                "build/kepubify*",
                "build/covergen*",
                "build/seriesmeta*",
            ],
        },
    }],
    "trigger": {
        "ref": [
            "refs/tags/v*",
        ],
        "event": {
            "exclude": ["promote"],
        },
    },
    "depends_on": [
        "kepub",
        "kepubify",
        "covergen",
        "seriesmeta",
    ],
}]

def main(ctx):
    return pipeline
