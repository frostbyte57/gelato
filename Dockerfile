FROM node:22-alpine AS web-build
WORKDIR /src/web
COPY web/package.json web/package-lock.json* ./
RUN npm install
COPY web/ ./
RUN npm run build

FROM golang:1.21-alpine AS go-build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web-build /src/web/dist ./web/dist
RUN go build -o /out/gelato ./cmd/gelato

FROM alpine:3.20
RUN adduser -D -u 10001 gelato
WORKDIR /app
COPY --from=go-build /out/gelato /usr/local/bin/gelato
COPY --from=web-build /src/web/dist ./web/dist
USER gelato
EXPOSE 8080
ENTRYPOINT ["gelato", "--assets-dir", "/app/web/dist"]

