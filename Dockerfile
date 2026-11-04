# 网页版服务器镜像：先用 Node 构建前端，再用 Go 把 dist 嵌入二进制，最终镜像只有一个可执行文件。
#   docker build -t bibliocell-web .
#   docker run -d -p 8080:8080 -v bibliocell-data:/data -e BIBLIOCELL_ADMIN_PASSWORD=change-me bibliocell-web
FROM node:22-alpine AS frontend
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build:web

FROM golang:1.27-alpine AS server
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /src/dist ./dist
RUN CGO_ENABLED=0 go build -tags webdist -ldflags="-s -w" -o /bibliocell .

FROM alpine:3.21
RUN adduser -D -u 1000 bibliocell && mkdir -p /data && chown bibliocell /data
USER bibliocell
VOLUME /data
# 数据目录：settings.json、bibliocell.sqlite 和 papers/ 都在这里。
ENV BIBLIOCELL_DATA_DIR=/data
EXPOSE 8080
ENTRYPOINT ["/bibliocell"]
CMD ["web", "-addr", "0.0.0.0:8080"]
