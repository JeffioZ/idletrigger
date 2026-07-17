# IdleTrigger

[English](README.md)

**轻量级 Windows 托盘工具：空闲动作、自动任务、保持唤醒和昼夜主题切换。**

IdleTrigger 是一个仅依赖 Windows 系统 DLL 的单文件程序。它驻留在通知区域，常用设置在浮层中完成，完整配置保存在 EXE 同目录的可读 TOML 文件中。

**[下载 x64（适合大多数电脑）](https://github.com/JeffioZ/idletrigger/releases/latest/download/IdleTrigger-x64.exe)** · [下载 x86（32 位 Windows）](https://github.com/JeffioZ/idletrigger/releases/latest/download/IdleTrigger-x86.exe) · [最新版说明与校验文件](https://github.com/JeffioZ/idletrigger/releases/latest)

## 适合这些场景

- 下载、渲染、备份或远程连接期间保持电脑唤醒，同时不改变平时使用的 Windows 睡眠设置。
- 键盘、鼠标持续无操作后，自动锁定、睡眠、休眠或关机。
- 在指定应用运行期间或特定时间段，自动启用或暂停保持唤醒和空闲监测。
- 按固定时间或日出日落自动切换 Windows 深浅色主题，并可选适配电池供电和全屏场景。

## 与系统设置及常见工具的区别

Windows 电源设置适合长期调整屏幕和睡眠超时；[PowerToys Awake](https://learn.microsoft.com/zh-cn/windows/powertoys/awake) 主要解决临时保持电脑唤醒。IdleTrigger 将保持唤醒、真实键鼠空闲后的动作，以及按时间或进程自动控制组合在一个便携托盘程序中。

它使用 Windows 电源请求和系统最后输入时间，不通过模拟鼠标或键盘操作维持运行。

## 系统要求

- Windows 10 / Windows Server 2016 及以上
- 大多数电脑使用 x64 构建；32 位 Windows 使用 x86 构建
- 昼夜主题依赖 Windows 个性化设置；Server Core 或被策略管理的桌面可能不可用

主构建不支持 Windows 7，兼容性原因见 [构建与开发指南](docs/development.zh-CN.md)。

## 快速开始

1. 新建一个准备长期保留且可写的目录，例如 `%LOCALAPPDATA%\IdleTrigger`。
2. 大多数电脑下载上方的 x64 版本；只有 32 位 Windows 才选择 x86。将 EXE 放入该目录。
3. 双击运行，程序会驻留在通知区域，不显示主窗口。
4. 左键托盘图标打开或关闭浮层；右键使用原生的“打开”和“退出”菜单。
5. 常用设置在浮层中完成；高级设置可编辑 EXE 同目录的 `IdleTrigger.toml`。

浮层会跟随 Windows 深浅色和 DPI 变化，直到手动关闭或再次左键托盘图标才会收起。每项功能入口均有 tooltip 说明。

### 升级或移动

替换 EXE 前先退出 IdleTrigger。保留 EXE 同目录的 `IdleTrigger.toml` 和 `IdleTrigger.state.json`，即可保留设置和自动任务状态。如果移动程序，请将这些文件一起移动，并从新位置启动一次后再依赖开机自启动。

## 使用控制浮层

- “电源管理”包含保持唤醒和空闲监测。两个开关显示手动设置；自动任务可以改变当前运行状态，但不会改写这些设置。
- “自动任务”显示已启用任务数和下次计划时间，可暂停全部任务，也可直接管理规则和进程条件，无需编辑 TOML。
- “系统控制”会立即执行；选择睡眠、休眠、关机或重启前，请先保存工作。
- 浮层支持鼠标和键盘操作；按 `Tab` / `Shift+Tab` 移动焦点，按 `Space` 激活当前控件。

## 界面截图

<img src="docs/images/control-panel-zh-CN-light.png" alt="IdleTrigger 简体中文浅色控制浮层" width="420">

<details>
<summary>更多主题与语言</summary>

| 简体中文深色 | 英文浅色 | 英文深色 |
| --- | --- | --- |
| <img src="docs/images/control-panel-zh-CN-dark.png" alt="IdleTrigger 简体中文深色控制浮层" width="260"> | <img src="docs/images/control-panel-en-light.png" alt="IdleTrigger 英文浅色控制浮层" width="260"> | <img src="docs/images/control-panel-en-dark.png" alt="IdleTrigger 英文深色控制浮层" width="260"> |

</details>

## 空闲监测

默认启用空闲监测，空闲时长为 30 分钟，动作为睡眠。浮层可选时长为：

`1、2、3、5、10、15、30 分钟；1、2、5 小时`。

空闲监测通过 Windows `GetLastInputInfo` 识别真实键盘和鼠标操作。程序刚启动或重新启用监测时会重新计时，不会因为启动前系统已经空闲而立刻执行；每次动作后也会重置计时。

“启用执行前提醒”开关打开后，动作前会显示不抢焦点的提示。鼠标、键盘操作或关闭提示都会取消本次动作；将 `idle_warning_seconds` 设为 `0` 可完全静默执行。

如果设备、驱动或应用反复刷新 Windows 空闲时间，导致空闲动作无法触发，可尝试“启用增强监测”。它会学习稳定的重置规律，同时仍将正常键鼠输入视为用户活动。

## 自动任务

通过“管理自动任务”即可创建规则，无需编辑 TOML。规则可以在指定进程运行期间或时间段内，临时启用或暂停保持唤醒和空闲监测；也可以按单次、每天、每周、进程启动或全部所选进程退出等条件，执行锁定、睡眠、休眠、关机或重启。

进程可按可执行文件名匹配，也可限定具体 EXE。程序每 5 秒采样一次进程状态：启动 IdleTrigger 时已经运行的进程不会补触发“启动”事件，完全发生在两次采样之间的短进程也可能被错过。退出规则会等待全部匹配实例关闭，并保留 5 秒宽限期以避免短暂重启造成误触发。

系统操作始终显示至少 10 秒、可取消的倒计时。任务只在 IdleTrigger 运行时生效，睡眠期间错过的计划不会在唤醒后补执行。规则只能调用内置动作，不能运行自定义命令或启动程序；进程匹配也不会注入代码、结束进程、安装服务或创建 Windows 任务计划。

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

改变 `nosleep` 或 `monitor` 状态的命令以及 `config:reload` 需要托盘程序正在运行；状态查询和一次性电源动作不受此限制。

## 配置

IdleTrigger 会在 EXE 同目录创建并维护 `IdleTrigger.toml`，同时保留已有的有效配置值。自动任务规则保存在其中，调度状态则单独保存在 `IdleTrigger.state.json`。

中英文字段说明见 [IdleTrigger.example.toml](IdleTrigger.example.toml)。修改会在数秒内自动应用；如需立即重载，可重启 IdleTrigger 或运行：

```powershell
.\IdleTrigger-x64.exe config:reload
```

开机自启保存在当前用户的 Windows Run 注册表项中，由浮层或 CLI 管理，不属于 TOML 配置。

## 日志

在浮层开启“调试日志”，或将 `logging_enabled = true`。日志优先写入 EXE 同目录，目录不可写时回退到 `%TEMP%`；文件达到 5 MiB 后轮转，保留一份旧日志，并在每行记录启动会话标识。

## 构建与开发

前置条件、双架构构建、资源生成和验证命令见 [构建与开发指南](docs/development.zh-CN.md)。

## 致谢

- [getlantern/systray](https://github.com/getlantern/systray)：Windows 托盘实现，基于 v1.2.2 调整（Apache-2.0）
- [BurntSushi/toml](https://github.com/BurntSushi/toml)：TOML 解析器
- [golang.org/x/sys](https://pkg.go.dev/golang.org/x/sys)：Windows API 绑定
- [NoSleep](https://github.com/CHerSun/NoSleep)：保持唤醒功能的设计灵感来源

## 许可证

MIT
