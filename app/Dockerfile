FROM golang:1.23
WORKDIR /app

# NOTE：外部パッケージのインストールを行うために追記
COPY go.mod go.sum ./
RUN go mod download

COPY app/main.go .
CMD go run main.go