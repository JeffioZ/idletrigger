# 构建指南

> [English](BUILD.md)

## 前置条件

- Go 1.25 或更高版本
- Git
- 仅重新生成图标时需要 Python 3
- 仅重新生成 Windows 资源时需要 `rsrc v0.10.2`

```powershell
go version
go mod download
```

IdleTrigger 支持 Windows 10 / Server 2016 及以上系统，同时支持
`windows/386` 和 `windows/amd64`。

当前构建不支持 Windows 7：Go 1.20 是最后一个可在 Windows 7 上运行的 Go
版本。若未来出现明确需求，应单独维护基于 Go 1.20 和兼容依赖版本的 legacy
构建，并在 Windows 7 实机或虚拟机上验证；不建议降低主构建的工具链版本。

## 构建

仓库同时包含 386 和 amd64 的 `.syso` 资源，因此两种架构构建均带有
应用图标和 manifest。

```powershell
$env:CGO_ENABLED = "0"
$env:GOARCH = "amd64" # 或 "386"
$version = "dev"
$ldflags = "-s -w -H windowsgui -X github.com/JeffioZ/idletrigger/internal/version.Value=$version"
$output = if ($env:GOARCH -eq "amd64") { "IdleTrigger-x64.exe" } else { "IdleTrigger-x86.exe" }
go build -trimpath -ldflags=$ldflags -o $output .
```

`CGO_ENABLED=0` 会生成只依赖 Windows 系统 DLL 的自包含 EXE；
`-H windowsgui` 用于避免托盘程序启动时闪出控制台窗口。

## 验证

```powershell
go test ./...
go vet ./...
gofmt -l .
go mod verify
```

发布工作流会先执行 test/vet，再构建两种架构，并随 EXE 发布
`SHA256SUMS.txt`。

## 离线构建

先在依赖可用的环境执行 `go mod vendor`，再把包含 `vendor/` 的仓库复制到
离线机器：

```powershell
$env:CGO_ENABLED = "0"
$env:GOARCH = "amd64"
go build -mod=vendor -trimpath -ldflags="-s -w -H windowsgui" -o IdleTrigger-x64.exe .
```

## 重新生成图标和资源

图标生成脚本只依赖 Python 标准库。各尺寸以 Windows 10 及以上系统原生支持的
PNG 压缩帧写入 ICO：

```powershell
python scripts/gen_icon.py assets
```

安装固定版本的资源编译器，并生成两种架构资源：

```powershell
go install github.com/akavel/rsrc@v0.10.2
rsrc -arch 386 -manifest assets/manifest.xml -ico assets/app.ico -o rsrc_windows_386.syso
rsrc -arch amd64 -manifest assets/manifest.xml -ico assets/app.ico -o rsrc_windows_amd64.syso
```

`app.ico`、三种托盘 ICO、生成脚本和两个 `.syso` 应一起提交，确保仓库中的
生成资源可以复现。

## 开发调试

```powershell
go test ./...
go build -o IdleTrigger-dev.exe .
./IdleTrigger-dev.exe

# 托盘运行后，在第二个终端中测试：
./IdleTrigger-dev.exe nosleep on
./IdleTrigger-dev.exe nosleep status
./IdleTrigger-dev.exe monitor on
```

UPX 压缩和代码签名属于可选发布步骤。调试构建不建议压缩，以免影响诊断和
杀毒软件分析。
