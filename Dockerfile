FROM docker.io/library/golang:1.17-alpine as builder

ARG GIT_COMMIT
ENV GIT_COMMIT=$GIT_COMMIT
ENV CGO_ENABLED=0

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -v -ldflags "-s -w -X main.version=$GIT_COMMIT" -trimpath -o ./build/kepubify ./cmd/kepubify

FROM scratch

COPY --from=builder /app/build/kepubify /opt/kepubify

ENTRYPOINT ["/opt/kepubify"]