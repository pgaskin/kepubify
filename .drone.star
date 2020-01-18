go = "1.13.6"

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
        ("test", "go test -mod=readonly ./kepub -v"),
    ], []),
    ("kepubify", [
        ("test", "go test -mod=readonly . -v"),
        ("run", "go run -mod=readonly . --help"),
    ], ["kepub"]),
    ("covergen", [
        ("test", "go test -mod=readonly ./covergen -v"),
        ("run", "go run -mod=readonly ./covergen --help"),
    ], []),
    ("seriesmeta", [
        ("test", "go test -mod=readonly ./seriesmeta -v"),
        ("run", "go run -mod=readonly ./seriesmeta --help"),
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
            "--build-cmd", "go env; CGO_ENABLED=%s go build -ldflags \"-s -w -X main.version=$(cat build/version)\" -o \"build/%s-%s\" %s" % ({
                False: "0",
                True: "1",
            }[cgo], app, {
                "windows/amd64": "windows-64bit.exe",
                "windows/386":   "windows-32bit.exe",
                "linux/amd64":   "linux-64bit",
                "linux/386":     "linux-32bit",
                "linux/armv6":   "linux-arm",
                "darwin/amd64":  "darwin-64bit",
            }[platform], path),
        ],
    } for (app, path, img, platform, cgo) in [
        ("kepubify", ".", "main",   "linux/amd64",   False),
        ("kepubify", ".", "main",   "linux/386",     False),
        ("kepubify", ".", "arm",    "linux/armv6",   False),
        ("kepubify", ".", "darwin", "darwin/amd64",  False),
        ("kepubify", ".", "main",   "windows/amd64", False),
        ("kepubify", ".", "main",   "windows/386",   False),

        ("covergen", "./covergen", "main",   "linux/amd64",   False),
        ("covergen", "./covergen", "main",   "linux/386",     False),
        ("covergen", "./covergen", "arm",    "linux/armv6",   False),
        ("covergen", "./covergen", "darwin", "darwin/amd64",  False),
        ("covergen", "./covergen", "main",   "windows/amd64", False),
        ("covergen", "./covergen", "main",   "windows/386",   False),

        ("seriesmeta", "./seriesmeta", "main",   "linux/amd64",   True),
        ("seriesmeta", "./seriesmeta", "main",   "linux/386",     True),
        ("seriesmeta", "./seriesmeta", "arm",    "linux/armv6",   True),
        ("seriesmeta", "./seriesmeta", "darwin", "darwin/amd64",  True),
        ("seriesmeta", "./seriesmeta", "main",   "windows/amd64", True),
        ("seriesmeta", "./seriesmeta", "main",   "windows/386",   True),
    ]] + [{
        "name": "debian",
        "image": "golang:%s-buster" % (go),
        "environment": {
            "GO111MODULE": "on",
        },
        "commands": [
            "go install github.com/goreleaser/nfpm/cmd/nfpm",
            "VERSION=$(cat build/debversion | tr -d v) nfpm pkg -f nfpm.yaml -t build/kepubify_$(cat build/debversion | tr -d v)_amd64.deb",
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
