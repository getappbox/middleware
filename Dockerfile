FROM golang:1.27.3-alpine AS build
RUN apk add --no-cache ca-certificates
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /middleware ./cmd/middleware

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
WORKDIR /app
COPY --from=build /middleware /app/middleware
USER 65532:65532
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 CMD ["/app/middleware", "--healthcheck", "--env", "production"]
ENTRYPOINT ["/app/middleware"]
CMD ["serve", "--env", "production", "--hostname", "0.0.0.0"]
