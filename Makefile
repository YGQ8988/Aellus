# Aellus 构建文件
# 用法:
#   make            # 交叉编译全平台全架构到 dist/
#   make clean      # 清理 dist/
#   make run        # 编译本机版本并运行
#   make vet        # 静态检查
#   make linux-arm64  # 单独编译某平台架构 (如 make windows-arm64)

APP      := aellus
DIST     := dist
LDFLAGS  := -s -w
VERSION  := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

# 全架构目标列表: <os>-<arch>
TARGETS  := \
	windows-amd64 windows-arm64 windows-386 \
	darwin-amd64 darwin-arm64 \
	linux-amd64 linux-arm64 linux-386 linux-arm

.PHONY: all clean vet run $(TARGETS)

all: $(TARGETS)
	@echo "✅ 编译完成:" && ls -lh $(DIST)/

# 自动生成每个目标的编译规则
# $(1)=target 如 linux-arm64; 解析出 GOOS/GOARCH, Windows 产物加 .exe
define BUILD_RULE
$(1):
	@mkdir -p $(DIST)
	@echo "→ $(word 1,$(subst -, ,$(1)))/$(word 2,$(subst -, ,$(1)))"
	@GOARM=7 CGO_ENABLED=0 GOOS=$(word 1,$(subst -, ,$(1))) GOARCH=$(word 2,$(subst -, ,$(1))) \
		go build -trimpath -ldflags="$(LDFLAGS) -X main.version=$(VERSION)" \
		-o $(DIST)/$(APP)-$(1)$(if $(filter windows-%,$(1)),.exe,) .
endef

$(foreach t,$(TARGETS),$(eval $(call BUILD_RULE,$(t))))

vet:
	@go vet ./...

run:
	@go run . --port 8000

clean:
	@rm -rf $(DIST)
	@echo "已清理 $(DIST)/"
