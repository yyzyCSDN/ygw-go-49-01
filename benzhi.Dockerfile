# Build-only image for the OCI artifact registry. The delivery branches are
# intentionally build-only: buggy baseline trees carry failing target tests,
# so this Dockerfile never runs `go test ./...` during the build. Container
# validation (go test/build/vet) is performed after the image is built, which
# is why the final image ships the Go toolchain and the offline vendor tree.
FROM --platform=$BUILDPLATFORM golang:1.23.12 AS builder
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
COPY vendor ./vendor
COPY cmd ./cmd
COPY internal ./internal
COPY web ./web
RUN CGO_ENABLED=0 GOARCH=$TARGETARCH go build -mod=vendor -trimpath -o /out/artifactregistry ./cmd/artifactregistry

FROM golang:1.23.12
ENV GOPROXY=off GOSUMDB=off GOFLAGS=-mod=vendor CGO_ENABLED=0
WORKDIR /app
COPY --from=builder /src/go.mod /src/go.sum /app/
COPY --from=builder /src/vendor /app/vendor
COPY --from=builder /src/cmd /app/cmd
COPY --from=builder /src/internal /app/internal
COPY --from=builder /src/web /app/web
COPY --from=builder /out/artifactregistry /usr/local/bin/artifactregistry
EXPOSE 8377
CMD ["/usr/local/bin/artifactregistry", "-addr", ":8377", "-browse", "/app/web/browse.html"]
