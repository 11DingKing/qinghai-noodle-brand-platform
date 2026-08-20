FROM golang:1.26 AS builder
ARG GOPROXY=https://proxy.golang.org,direct
ENV GOTOOLCHAIN=local
ENV GOPROXY=${GOPROXY}
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /qinghainoodle ./cmd/qinghainoodle
RUN CGO_ENABLED=0 go build -o /brandctl ./cmd/brandctl

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /qinghainoodle /qinghainoodle
COPY --from=builder /brandctl /brandctl
EXPOSE 49660
ENTRYPOINT ["/qinghainoodle"]
