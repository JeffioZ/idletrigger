# 构建指南

> 📖 [English](BUILD.md)

## 前置条件

- **Go 1.21+** — [下载](https://go.dev/dl/)
- **Git** — 用于依赖解析（也可使用 `GOPROXY`）

验证安装：

```
go version   # → go version go1.21+ windows/amd64
```

## 一键构建

```powershell
cd IdleTrigger
$env:CGO_ENABLED = "0"
go mod tidy        # 首次需下载依赖
go build -ldflags="-s -w -H windowsgui" -o IdleTrigger.exe .
```

| 参数 / 环境变量 | 作用 |
|-----------------|------|
| `CGO_ENABLED=0` | **强制纯 Go 静态链接** — 不依赖 C 运行时，无外部 DLL |
| `-s` | 去除符号表 |
| `-w` | 去除 DWARF 调试信息 |

三者配合生成**单一自包含 EXE 文件**（约 5 MB）。
唯一的"依赖"是 Windows 内置 DLL（kernel32、user32 等），
从 XP 起每个 Windows 都自带。

## 进一步压缩（可选）

Go 构建完成后，使用 [UPX](https://upx.github.io/) 进行极限压缩：

```powershell
upx --best --lzma IdleTrigger.exe
```

通常可将二进制压缩至 **~1.5 MB**。

## 依赖项

| 包 | 用途 |
|----|------|
| `github.com/BurntSushi/toml` | TOML 配置文件解析 |
| `github.com/getlantern/systray` | 跨平台系统托盘 |
| `golang.org/x/sys` | Windows API 绑定 |

所有依赖在 `go build` / `go mod tidy` 时自动下载解析。

注：Windows XP 兼容需使用 Go ≤ 1.20 及合适的 C 工具链。
Go 1.21+ 最低支持 Windows 7（或 Windows Server 2008 R2）。

## 离线/断网环境构建

```powershell
go mod vendor     # 将依赖缓存到 vendor 目录
go build -mod=vendor -ldflags="-s -w" -o IdleTrigger.exe .
```

## 单 EXE 保证

IdleTrigger 是**真正的单文件可执行程序**：

- **纯 Go** — `CGO_ENABLED=0` 彻底禁用 CGo；无需 MinGW，无需 MSVC 运行时
- **静态链接** — 所有 Go 包和内嵌资源编译进同一个 `.exe`
- **仅调用系统 DLL** — 二进制只调用 Windows 内置 DLL（kernel32.dll、
  user32.dll、powrprof.dll、advapi32.dll），从 XP 起每个 Windows 都自带
- **资源内嵌** — 托盘图标和语言文件通过 `//go:embed` 编译进 EXE

✅ 把 `IdleTrigger.exe` 复制到任意 Windows 电脑的任意文件夹，双击即用。
无需安装、无需运行时、无需额外文件。

## 生成新的托盘图标

编辑 `scripts/gen_icon.py`，然后运行：

```powershell
python scripts/gen_icon.py assets/
```

重新构建 EXE 即可嵌入新图标。

## 开发调试

```powershell
# 构建并运行 GUI 模式
go build -o IdleTrigger.exe . && .\IdleTrigger.exe

# 构建并运行 CLI 模式
go build -o IdleTrigger.exe . && .\IdleTrigger.exe lock

# 测试 IPC 通信（需先在另一个终端中运行托盘）
.\IdleTrigger.exe nosleep on
.\IdleTrigger.exe nosleep status
.\IdleTrigger.exe monitor on
```
