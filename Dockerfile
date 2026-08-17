FROM node:24-alpine AS web-build
WORKDIR /src
COPY web/package.json web/package-lock.json ./web/
RUN npm --prefix web ci
COPY web ./web
RUN npm --prefix web run build

FROM golang:1.26.6-alpine AS go-build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web-build /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/lapin ./cmd/lapin

FROM alpine:3.22
RUN addgroup -S lapin && adduser -S -G lapin lapin \
    && mkdir -p /var/lib/lapin/assets \
    && chown lapin:lapin /var/lib/lapin/assets
COPY --from=go-build /out/lapin /usr/local/bin/lapin
USER lapin
EXPOSE 8080
ENTRYPOINT ["lapin"]
