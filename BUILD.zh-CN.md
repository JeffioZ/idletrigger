# 构建 IdleTrigger

[English](BUILD.md)

## 前置条件

- Go 1.26 或更高版本
- Git
- 仅重新生成 Windows 资源时需要 `github.com/akavel/rsrc`

IdleTrigger 面向 Windows 10 / Windows Server 2016 及以上系统，仓库同时产出 `windows/amd64` 和 `windows/386`。

主构建不支持 Windows 7。Go 1.20 是最后一个可运行于 Windows 7 的 Go 版本；如未来确有需求，应独立维护基于 Go 1.20、使用兼容依赖版本的 legacy 构建，并完成实机验证。

```powershell
go version
go mod download
```

## 构建

仓库中的架构专用 `.syso` 文件包含应用图标、manifest 与 Windows 版本信息。发布版本号变化时，必须重新生成资源，使资源管理器属性页与应用版本保持一致。

```powershell
$env:CGO_ENABLED = "0"
$env:GOARCH = "amd64" # 32 位 Windows 使用 "386"
$version = "dev"
$ldflags = "-s -w -H windowsgui -X github.com/JeffioZ/idletrigger/internal/version.Value=$version"
$output = if ($env:GOARCH -eq "amd64") { "IdleTrigger-x64.exe" } else { "IdleTrigger-x86.exe" }
go build -trimpath "-ldflags=$ldflags" -o $output .
```

`CGO_ENABLED=0` 使 EXE 保持自包含；`-H windowsgui` 避免托盘程序启动时显示控制台窗口。

## 验证

发布前执行：

```powershell
go test -count=1 ./...
go vet ./...
gofmt -l .
go mod verify
```

并显式构建两种架构：

```powershell
$env:CGO_ENABLED = "0"
$version = "dev"
$ldflags = "-s -w -H windowsgui -X github.com/JeffioZ/idletrigger/internal/version.Value=$version"

$env:GOARCH = "amd64"
go build -trimpath "-ldflags=$ldflags" -o dist/IdleTrigger-x64.exe .

$env:GOARCH = "386"
go build -trimpath "-ldflags=$ldflags" -o dist/IdleTrigger-x86.exe .
```

发布工作流会先运行格式、依赖、test 与 vet 检查，再生成两种 EXE，并发布 `SHA256SUMS.txt`。

## 重新生成资源

更新 `assets/app.ico` 后，先生成相应的托盘图标变体：

```powershell
go run ./scripts/gen_tray_theme_icons assets
```

然后重新生成两种架构资源。资源命令与发布构建必须使用同一个版本号：

```powershell
$version = "1.3.0"
go run ./scripts/gen_resource.go -version $version
```

请将 `app.ico`、两个托盘 ICO、`assets/manifest.xml`、生成器和两份 `.syso` 一并提交，确保发布资源可复现。

## 离线构建

先在联网环境 vendor 依赖，再将包含 `vendor/` 的仓库复制到离线机器：

```powershell
go mod vendor

$env:CGO_ENABLED = "0"
$env:GOARCH = "amd64"
go build -mod=vendor -trimpath -ldflags="-s -w -H windowsgui" -o dist/IdleTrigger-x64.exe .
```

## 开发调试

```powershell
go test ./...
$env:CGO_ENABLED = "0"
$env:GOARCH = "amd64"
go build -trimpath -ldflags="-H windowsgui" -o dist/IdleTrigger-x64-dev.exe .
.\dist\IdleTrigger-x64-dev.exe

# 托盘程序启动后，在第二个终端中执行：
cmd /c .\dist\IdleTrigger-x64-dev.exe nosleep on
cmd /c .\dist\IdleTrigger-x64-dev.exe nosleep status
cmd /c .\dist\IdleTrigger-x64-dev.exe monitor on
```

开发构建使用 Windows GUI 子系统，避免启动托盘程序时闪出控制台窗口。验证 CLI
输出时，请通过 `cmd /c` 执行，或用 `Start-Process` 重定向 stdout/stderr；
PowerShell 直接运行 GUI 子系统 EXE 时，可能在输出完成绑定前就返回。

制作文档截图时，可在启动 EXE 前设置 `IDLETRIGGER_CAPTURE_MODE=1`，再从托盘图标打开浮层。截图模式会把控制浮层显示为普通顶层应用窗口，便于截图工具选中整窗；该模式仅用于文档截图和视觉检查。

代码签名是可选发布步骤。调试构建不要使用 UPX 加壳，以免增加诊断和杀毒软件分析成本。
