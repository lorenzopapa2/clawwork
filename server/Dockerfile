FROM golang:1.24-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -o clawwork-server .

FROM alpine:3.20

RUN apk add --no-cache ca-certificates

WORKDIR /app
COPY --from=builder /app/clawwork-server .

EXPOSE 8080

ENV PORT=8080
ENV DB_PATH=/data/clawwork.db

CMD ["./clawwork-server"]
