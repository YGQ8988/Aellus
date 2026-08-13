# Aellus 构建文件
# 用法:
#   make            # 交叉编译三端到 dist/
#   make clean      # 清理 dist/
#   make run        # 编译本机版本并运行
#   make vet        # 静态检查

APP      := aellus
DIST     := dist
LDFLAGS  := -s -w
TARGETS  := windows-amd64 linux-amd64 darwin-amd64 darwin-arm64

.PHONY: all clean vet run $(TARGETS)

all: $(TARGETS)
	@echo "✅ 编译完成:" && ls -lh $(DIST)/

# 交叉编译规则: make windows-amd64 等
windows-amd64:
	@mkdir -p $(DIST)
	@echo "→ windows/amd64"
	@CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="$(LDFLAGS)" -o $(DIST)/$(APP)-windows-amd64.exe .

linux-amd64:
	@mkdir -p $(DIST)
	@echo "→ linux/amd64"
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="$(LDFLAGS)" -o $(DIST)/$(APP)-linux-amd64 .

darwin-amd64:
	@mkdir -p $(DIST)
	@echo "→ darwin/amd64"
	@CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags="$(LDFLAGS)" -o $(DIST)/$(APP)-darwin-amd64 .

darwin-arm64:
	@mkdir -p $(DIST)
	@echo "→ darwin/arm64"
	@CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="$(LDFLAGS)" -o $(DIST)/$(APP)-darwin-arm64 .

vet:
	@go vet ./...

run:
	@go run . --port 8000

clean:
	@rm -rf $(DIST)
	@echo "已清理 $(DIST)/"
