.PHONY: dev build frontend

# 全栈开发（后端 air 热重载 + 前端 Vite HMR）
dev:
	@echo "启动 air 热重载..."
	~/go/bin/air

# 前端开发（热更新）
dev-frontend:
	cd web && npm run dev

# 后端开发（无热重载）
dev-backend:
	go run .

# 构建前端
frontend:
	cd web && npm run build

# 构建完整二进制
build: frontend
	CGO_ENABLED=0 go build -o okb-web .

# 清理
clean:
	rm -f okb-web
	rm -rf web/dist
