# IdleTrigger

> 📖 [English](README.md)

**Windows 系统空闲监测、电源管理及防休眠工具**

轻量级单文件工具，驻留系统托盘。它可以：

- **阻止休眠** — 保持系统唤醒，重置 Windows 空闲计时器
- **自动触发** — 空闲超时后自动执行睡眠 / 休眠 / 关机 / 锁屏
- **命令行调用** — 从终端、脚本或 AI agent 触发动作
- **进程间通信** — CLI 命令与运行中的托盘实例无缝通信

## 功能特性

- **系统托盘** — 右键菜单完成全部设置，无窗口
- **保持唤醒** — 阻止 Windows 自动休眠，可选屏幕常亮
- **空闲监测** — 键鼠无操作 N 分钟后自动触发
  （5 / 10 / 30 / 60 / 120 分钟，托盘子菜单可配）
- **全局热键** — `Win+Shift+S` 睡眠 / `+L` 锁屏 / `+N` 切换保持唤醒
- **电池感知** — 使用电池时自动禁用保持唤醒
- **进程关联** — 检测到指定应用运行时自动启用保持唤醒
- **IPC 命名管道** — CLI 与运行中的托盘实例通信
- **能力检测** — 不可用的功能（如休眠）自动禁用
- **单文件 EXE** — 全静态链接；复制即用，免安装
- **明文配置** — `IdleTrigger.toml` 位于 EXE 同目录
- **多语言** — 中文 / English。默认跟随系统语言（中文 Windows 显示中文，其余显示英文），托盘菜单可手动切换
- **DPI 和深色模式** — Per-Monitor V2，原生深色菜单和对话框
- **系统支持**：Windows 7 及以上，32 位 / 64 位。32 位 EXE 可在两种架构上运行

## 快速开始

1. 从 [Releases](https://github.com/JeffioZ/idletrigger/releases) 下载 `IdleTrigger.exe`
2. 双击运行 → 托盘出现石板蓝色电源图标
3. 右键配置；或直接编辑 `IdleTrigger.toml`

## CLI 命令行用法

```
IdleTrigger sleep                 进入睡眠
IdleTrigger hibernate             进入休眠
IdleTrigger shutdown              关闭系统
IdleTrigger lock                  锁定屏幕

IdleTrigger nosleep on            保持系统唤醒
IdleTrigger nosleep on --screen   保持唤醒 + 屏幕常亮
IdleTrigger nosleep off           恢复正常电源管理
IdleTrigger nosleep toggle        切换保持唤醒
IdleTrigger nosleep status        查看保持唤醒状态

IdleTrigger monitor on            启用空闲监测（IPC → 托盘）
IdleTrigger monitor off           禁用空闲监测（IPC → 托盘）

IdleTrigger autostart enable      启用开机自启
IdleTrigger autostart disable     禁用开机自启
IdleTrigger autostart status      查看开机自启状态

IdleTrigger status                查看完整系统状态
IdleTrigger version               显示版本
```

不带参数运行即启动系统托盘 GUI 模式。

**AI agent / 脚本集成**：托盘运行时，`nosleep` 和 `monitor` 命令
通过命名管道（`\\.\pipe\IdleTrigger`）转发。一次性动作直接执行。

## 配置文件 (`IdleTrigger.toml`)

```toml
# IdleTrigger 配置文件
# 可直接编辑；重启生效（或通过 CLI "config:reload" 热加载）。

language = "auto"                  # "auto"（跟随系统）, "en", "zh-CN"
idle_timeout_minutes = 30          # 0 = 禁用空闲监测
idle_action = "sleep"              # sleep | hibernate | shutdown | lock
idle_warning_seconds = 30          # 触发前通知秒数；0 = 关闭

nosleep_enabled = false            # 保持唤醒
keep_screen_on = false             # 同步保持屏幕常亮
nosleep_on_battery = false         # 电池供电时仍保持唤醒
nosleep_battery_threshold = 20     # 最低电量百分比

hotkeys_enabled = false            # Win+Shift+S/L/N

process_watch_enabled = false      # 进程关联自动唤醒
process_watch_list = []            # 例如 ["chrome.exe", "powerpnt.exe"]

start_minimized = true
logging_enabled = false            # 调试日志输出

theme_switch_enabled = false       # 自动主题切换
theme_mode = "sunrise"             # "fixed" 或 "sunrise"
theme_light_time = "07:00"
theme_dark_time = "19:00"
theme_latitude = 0                 # 0 = 根据时区自动检测
theme_longitude = 0
theme_dark_on_battery = true
theme_skip_fullscreen = true

autostart_enabled = false
```

## 项目结构

```
IdleTrigger/
├── main.go                          # 入口：CLI / GUI 双模式调度
├── assets/
│   ├── app.ico                      # EXE 图标（16/32/48/256）
│   ├── icon.go                      # go:embed 内嵌桥接
│   ├── icon_default.ico             # 托盘：待命状态（石板蓝）
│   ├── icon_monitor.ico             # 托盘：监测中（琥珀色）
│   ├── icon_active.ico              # 托盘：保持唤醒中（绿色）
│   ├── manifest.xml                 # DPI 和深色模式清单
│   └── resource.rc                  # Windows 资源脚本
├── scripts/
│   └── gen_icon.py                  # 图标生成脚本（仅开发用）
├── internal/
│   ├── actions/actions.go           # Win32 系统动作
│   ├── autostart/autostart.go       # 注册表 Run 键管理
│   ├── cli/cli.go                   # CLI 命令分发 + IPC 客户端
│   ├── config/config.go             # TOML 配置读写
│   ├── darkmode/darkmode.go         # uxtheme 序号 135/136
│   ├── dialog/dialog.go             # TaskDialog（深色适配）
│   ├── dpi/dpi.go                   # Per-Monitor V2
│   ├── hotkey/hotkey.go             # 全局热键
│   ├── i18n/                        # 多语言
│   │   ├── i18n.go
│   │   └── locales/{en,zh-CN}.json
│   ├── ipc/ipc.go                   # 命名管道 IPC
│   ├── monitor/monitor.go           # GetLastInputInfo 空闲检测
│   ├── nosleep/nosleep.go           # SetThreadExecutionState
│   ├── notify/notify.go             # 气泡通知
│   ├── power/power.go               # 电池状态 + 睡眠能力检测
│   ├── processwatcher/processwatcher.go  # 进程列表监测
│   └── tray/tray.go                 # 系统托盘菜单 + IPC 服务端
├── rsrc_windows_amd64.syso          # 编译后的资源文件
├── go.mod  go.sum  LICENSE  .gitattributes  .gitignore
├── README.md  README.zh-CN.md  BUILD.md  BUILD.zh-CN.md
```

## 托盘菜单参考

```
睡眠 / 休眠 / 关机 / 锁屏
─────────────────
保持唤醒
  屏幕常亮
─────────────────
空闲监测
  超时时间 ▸  5 / 10 / 30 / 60 / 120 分钟
  触发动作 ▸  睡眠 / 休眠 / 关机 / 锁屏
─────────────────
全局热键
开机自启
语言 ▸  English / 中文
─────────────────
打开配置文件
关于
─────────────────
退出
```

## 致谢

- [getlantern/systray](https://github.com/getlantern/systray) — 跨平台系统托盘库
- [BurntSushi/toml](https://github.com/BurntSushi/toml) — Go 语言 TOML 解析器
- [golang.org/x/sys](https://golang.org/x/sys) — Windows API 绑定

## 许可证

MIT
