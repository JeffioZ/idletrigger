# 构建 IdleTrigger

[English](development.md)

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

架构专用 `.syso` 文件包含应用图标、manifest 与 Windows 版本信息。它们是生成型构建产物，不提交到仓库。构建前先重新生成资源，使资源管理器属性页与应用版本保持一致。

```powershell
$env:CGO_ENABLED = "0"
$env:GOARCH = "amd64" # 32 位 Windows 使用 "386"
$version = "dev"
go run ./tools/resourcegen.go -version $version
$ldflags = "-s -w -H windowsgui -X github.com/JeffioZ/idletrigger/internal/version.Value=$version"
$output = if ($env:GOARCH -eq "amd64") { "IdleTrigger-x64.exe" } else { "IdleTrigger-x86.exe" }
go build -trimpath "-ldflags=$ldflags" -o $output ./cmd/idletrigger
```

`CGO_ENABLED=0` 使 EXE 保持自包含；`-H windowsgui` 避免托盘程序启动时显示控制台窗口。

## 验证

在 Windows PowerShell 5.1 或更高版本中执行本地标准检查：

```powershell
.\tools\check.ps1
```

检查脚本会覆盖普通、`devtools` 和 `tools` 三种构建标签组合。已安装
`golangci-lint` 时脚本会执行它；未安装时会明确打印 `SKIPPED`。
联网时可执行可选漏洞扫描：

```powershell
.\tools\check.ps1 -Vulncheck
```

未传 `-Vulncheck` 时不会执行漏洞扫描，脚本会明确提示。若系统缓存目录不可写，
可在当前 shell 中先设置以下可选的用户本地缓存路径：

```powershell
$env:GOCACHE = Join-Path $env:LOCALAPPDATA "IdleTrigger\cache\go-build"
$env:GOLANGCI_LINT_CACHE = Join-Path $env:LOCALAPPDATA "IdleTrigger\cache\golangci-lint"
```

并显式构建两种架构：

```powershell
$env:CGO_ENABLED = "0"
$version = "dev"
go run ./tools/resourcegen.go -version $version
$ldflags = "-s -w -H windowsgui -X github.com/JeffioZ/idletrigger/internal/version.Value=$version"

$env:GOARCH = "amd64"
go build -trimpath "-ldflags=$ldflags" -o dist/IdleTrigger-x64.exe ./cmd/idletrigger

$env:GOARCH = "386"
go build -trimpath "-ldflags=$ldflags" -o dist/IdleTrigger-x86.exe ./cmd/idletrigger
```

发布工作流会先运行格式、依赖、test 与 vet 检查，再生成两种 EXE，并发布 `SHA256SUMS.txt`。

## 重新生成资源

主图标与两套托盘图标采用独立图稿。更新时先生成主 ICO，再生成专门适配任务栏的托盘变体：

```powershell
go run ./tools/appicon/main.go build/windows/icons
go run ./tools/trayicons/main.go build/windows/icons
```

然后重新生成两种架构资源。资源命令与发布构建必须使用同一个版本号：

```powershell
$version = "1.3.0"
go run ./tools/resourcegen.go -version $version
```

请将 `app.ico`、两个托盘 ICO、`build/windows/manifest.xml` 和生成器一并提交。不要提交 `.syso` 文件；发布工作流会按 tag 版本自动重新生成。

## 重新生成 README 截图

截图生成属于维护能力，只在使用 `devtools` 构建标签时编译。辅助脚本会临时构建
devtools EXE、重新生成四张受版本管理的图片、校验 PNG 尺寸，并删除临时构建目录：

```powershell
.\tools\capture-screenshots.ps1
```

如只想验证截图流程而不覆盖仓库图片，可指定临时输出目录：

```powershell
.\tools\capture-screenshots.ps1 -OutputDirectory (Join-Path $env:TEMP "IdleTrigger-screenshots")
```

正式 EXE 明确不包含 `screenshot` 命令及其 PNG/压缩依赖。

## 离线构建

先在联网环境 vendor 依赖，再将包含 `vendor/` 的仓库复制到离线机器：

```powershell
go mod vendor

$env:CGO_ENABLED = "0"
$env:GOARCH = "amd64"
go run ./tools/resourcegen.go -version dev
go build -mod=vendor -trimpath -ldflags="-s -w -H windowsgui" -o dist/IdleTrigger-x64.exe ./cmd/idletrigger
```

## 开发调试

```powershell
go test ./...
go run ./tools/resourcegen.go -version dev
$env:CGO_ENABLED = "0"
$env:GOARCH = "amd64"
go build -tags devtools -trimpath -ldflags="-H windowsgui" -o dist/IdleTrigger-x64-devtools.exe ./cmd/idletrigger
.\dist\IdleTrigger-x64-devtools.exe

# 托盘程序启动后，在第二个终端中执行：
cmd /c .\dist\IdleTrigger-x64-devtools.exe nosleep on
cmd /c .\dist\IdleTrigger-x64-devtools.exe nosleep status
cmd /c .\dist\IdleTrigger-x64-devtools.exe monitor on
cmd /c .\dist\IdleTrigger-x64-devtools.exe diagnostics idle
```

开发构建使用 Windows GUI 子系统，避免启动托盘程序时闪出控制台窗口。验证 CLI
输出时，请通过 `cmd /c` 执行，或用 `Start-Process` 重定向 stdout/stderr；
PowerShell 直接运行 GUI 子系统 EXE 时，可能在输出完成绑定前就返回。
`diagnostics`、截图和本机测试环境变量等维护能力只存在于 devtools 构建。

代码签名是可选发布步骤。调试构建不要使用 UPX 加壳，以免增加诊断和杀毒软件分析成本。
