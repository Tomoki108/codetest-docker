FROM golang:1.23
WORKDIR /app

# NOTE: GOPATH（/go）直下にある go.modを明示的に無視して警告を消す
ENV GO111MODULE=on

# NOTE：外部パッケージのインストールを行うために追記
COPY go.mod go.sum ./
RUN go mod download

COPY app/main.go .
CMD go run main.go