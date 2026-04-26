FROM golang:1.22 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /podlabeler .

FROM gcr.io/distroless/static:nonroot AS runner
COPY --from=builder /podlabeler /podlabeler
ENTRYPOINT ["/podlabeler"]
