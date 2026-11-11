FROM --platform=$BUILDPLATFORM golang:1.26.10 AS build
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -trimpath -o /out/x-mcp ./cmd/x-mcp

FROM gcr.io/distroless/static-debian12
WORKDIR /app
COPY --from=build --chown=65532:65532 /out/x-mcp /app/x-mcp
EXPOSE 8080
USER 65532:65532
ENTRYPOINT ["/app/x-mcp"]
