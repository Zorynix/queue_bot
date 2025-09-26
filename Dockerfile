
FROM golang:1.25-alpine AS builder

WORKDIR /app

RUN apk add --no-cache ca-certificates git tzdata

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOEXPERIMENT=greenteagc go build -a -installsuffix cgo -o queue-bot .

FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/queue-bot .

COPY queue_lessons.txt ./
COPY user_mapping.json ./
COPY .env ./

RUN mkdir -p /app/credentials


EXPOSE 8080

CMD ["./queue-bot"]