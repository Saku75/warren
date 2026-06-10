# Build stage. Generated code (templ) is committed, so the build needs only
# the Go toolchain.
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/warren ./cmd/warren

# Runtime stage: distroless, non-root, no shell. The binary embeds all
# assets, so the image needs nothing else and runs with a read-only root
# filesystem.
FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=build /out/warren /warren
EXPOSE 8080
ENTRYPOINT ["/warren"]
CMD ["serve"]
