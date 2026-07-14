# IdleTrigger

[English](README.md)

**轻量级 Windows 托盘工具：空闲动作、保持唤醒和昼夜主题切换。**

IdleTrigger 是一个仅依赖 Windows 系统 DLL 的单文件程序。它驻留在通知区域，常用设置在浮层中完成，完整配置保存在 EXE 同目录的可读 TOML 文件中。

## 能做什么

- **空闲监测**：键盘、鼠标无操作达到设定时长后，锁定、睡眠、休眠或关机。
- **执行前提醒**：动作前显示不抢焦点的提醒；移动鼠标、按任意键或关闭提醒即可取消本次动作。
- **保持唤醒**：阻止系统自动睡眠，并可选保持屏幕常亮。
- **昼夜模式**：按固定时间或日出日落切换 Windows 深浅色主题；可选电池供电时使用深色。
- **系统控制**：从浮层或命令行执行锁定、睡眠、休眠、关机、重启。
- **自动化控制**：通过当前会话的命名管道控制已运行的托盘实例。

## 系统要求

- Windows 10 / Windows Server 2016 及以上
- 大多数电脑使用 x64 构建；32 位 Windows 使用 x86 构建
- 昼夜主题依赖 Windows 个性化设置；Server Core 或被策略管理的桌面可能不可用

主构建不支持 Windows 7，兼容性原因见 [BUILD.zh-CN.md](BUILD.zh-CN.md)。

## 快速开始

1. 从 [Releases](https://github.com/JeffioZ/idletrigger/releases) 下载 `IdleTrigger-x64.exe`。
2. 双击运行，程序会驻留在通知区域，不显示主窗口。
3. 左键托盘图标打开或关闭浮层；右键使用原生的“打开”和“退出”菜单。
4. 常用设置在浮层中完成；高级设置可编辑 EXE 同目录的 `IdleTrigger.toml`。

浮层会跟随 Windows 深浅色和 DPI 变化，直到手动关闭或再次左键托盘图标才会收起。每项功能入口均有 tooltip 说明。

## 使用控制浮层

- 蓝色表示已启用或已选中；中性色表示可用但未选中。“退出”为红色，因为它会停止 IdleTrigger 的全部功能。
- 悬停“系统控制”或“语言设置”即可展开对应菜单。“系统控制”会立即执行；选择睡眠、休眠、关机或重启前，请先保存工作。
- 可使用鼠标，或按 `Tab` / `Shift+Tab` 在控件间移动，再按 `Space` 激活当前控件；键盘焦点会显示清晰的轮廓。
- 紧凑浮层只承载日常操作；适用进程、位置和详细主题规则等高级设置请通过“编辑配置”处理。

## 界面截图

<img src="docs/images/panel-zh-light.png" alt="IdleTrigger 简体中文浅色控制浮层" width="420">

<details>
<summary>更多主题与语言</summary>

| 简体中文深色 | 英文浅色 | 英文深色 |
| --- | --- | --- |
| <img src="docs/images/panel-zh-dark.png" alt="IdleTrigger 简体中文深色控制浮层" width="260"> | <img src="docs/images/panel-en-light.png" alt="IdleTrigger 英文浅色控制浮层" width="260"> | <img src="docs/images/panel-en-dark.png" alt="IdleTrigger 英文深色控制浮层" width="260"> |

</details>

## 空闲监测

默认启用空闲监测，空闲时长为 30 分钟，动作为睡眠。浮层可选时长为：

`1、2、3、5、10、15、30 分钟；1、2、5 小时`。

空闲监测通过 Windows `GetLastInputInfo` 识别真实键盘和鼠标操作。程序刚启动、重新启用监测时会从新的空闲窗口开始，不会因为启动前系统已经空闲而立刻执行。动作触发后会先重置空闲窗口，再继续监测。

开启“执行前显示提醒”后，动作前会显示不抢焦点的提示。鼠标、键盘操作或关闭提示都会取消本次动作；将 `idle_warning_seconds` 设为 `0` 可完全静默执行。

如果设备、驱动或应用让 Windows 空闲时间按固定间隔刷新，导致系统睡眠或空闲动作无法触发，可开启“增强空闲监测”。该开关默认关闭；开启后 IdleTrigger 会先记录并学习稳定周期，再用更稳健的方式累计空闲时间。普通键鼠操作仍会正常重置计时，日志也会记录每次重置被接受或忽略的原因。

## 命令行

不带参数运行 EXE 即启动托盘程序。

```text
IdleTrigger sleep | hibernate | shutdown | restart | lock

IdleTrigger nosleep on [--screen]
IdleTrigger nosleep off | toggle | status

IdleTrigger monitor on | off | status

IdleTrigger autostart enable | disable | status
IdleTrigger config:reload
IdleTrigger status
IdleTrigger version
```

改变 `nosleep` 或 `monitor` 状态的命令以及 `config:reload` 会通过 `\\.\pipe\IdleTrigger-<session>` 转发给运行中的托盘实例，因此需要先启动托盘程序。状态查询在托盘程序未运行时仍会返回结果；一次性电源动作直接执行。

## 配置

IdleTrigger 会在 EXE 同目录创建并维护 `IdleTrigger.toml`。配置模板更新时，程序会补齐缺失字段、更新说明注释，并保留已有的有效配置值；不会在每次启动时重复改写文件。

完整的中英文字段说明见 [IdleTrigger.example.toml](IdleTrigger.example.toml)。保存修改后会在数秒内自动应用；如需立即生效，可重启 IdleTrigger 或运行：

```powershell
.\IdleTrigger-x64.exe config:reload
```

开机自启保存在当前用户的 Windows Run 注册表项中，由浮层或 CLI 管理，不属于 TOML 配置。

## 日志

在浮层开启“调试日志”，或将 `logging_enabled = true`。日志优先写入 EXE 同目录，目录不可写时回退到 `%TEMP%`；文件达到 5 MiB 后轮转，并保留一份 `IdleTrigger.log.1`。

每行日志包含启动会话标识，便于区分不同运行周期：

```text
[2026-07-11 12:34:56.789] [session:18a0f0-2b4c] Idle monitor started
```

## 构建与开发

前置条件、双架构构建、资源生成和验证命令见 [BUILD.zh-CN.md](BUILD.zh-CN.md)。

## 项目结构

```text
assets/                  应用图标、manifest、托盘图标变体和资源工具
docs/images/             README 使用的界面截图
internal/actions/        锁定、睡眠、休眠、关机、重启等 Windows 动作
internal/config/         TOML 读取、校验、迁移和原子保存
internal/idlewarning/    不抢焦点、支持 DPI 的空闲预警浮层
internal/monitor/        键盘/鼠标空闲跟踪和触发生命周期
internal/popup/          原生、支持 DPI 的控制浮层
internal/systray/        本地 Windows 通知区域实现
internal/themeswitch/    固定时间和日出日落主题调度
internal/tray/           串行应用状态和功能协调
scripts/                 资源和主题托盘图标生成器
```

## 致谢

- [getlantern/systray](https://github.com/getlantern/systray)：本地 Windows 托盘实现基于 v1.2.2 派生，并按 IdleTrigger 需求调整（Apache-2.0）
- [BurntSushi/toml](https://github.com/BurntSushi/toml)：TOML 解析器
- [golang.org/x/sys](https://pkg.go.dev/golang.org/x/sys)：Windows API 绑定
- [NoSleep](https://github.com/CHerSun/NoSleep)：保持唤醒功能的设计灵感来源

## 许可证

MIT
