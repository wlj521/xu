# Xu

> 一个类 FinalShell 的 Windows SSH 工作台：原生桌面 UI、终端、SFTP、服务器监控与端口转发，不使用 WebView。

- 版本：`0.1.0`
- 技术栈：Go `1.27.3` + [mygo](https://github.com/egoist/mygo) `0.8.3` 原生 UI + Ghostty VT
- 平台：Windows 10 1809+ / Windows 11（x64）

> **打包命令：**
>
> ```powershell
> .\build-local.ps1
> ```

---

## 功能

- **会话管理**：用户自建分组、保存 SSH 连接、密码/私钥认证、主机密钥 TOFU 校验、快速连接与连接管理器；同一服务器可开多个标签页（自动编号区分）。
- **终端**：多标签页 SSH 终端与本地 PowerShell 终端（顶部标签栏切换/关闭，同一服务器可开多个标签）、查找、清屏、断线重连。
- **文件**：底部 FinalShell 风格远端 SFTP 面板：左侧目录树、面包屑路径、文件名/大小/类型/修改时间/权限/用户组列表；默认从 `/` 打开，单击目录进入下一层，面板高度可上下拖动（双击分隔条复位），支持上传、下载、新建文件/文件夹、重命名、删除、远程文本编辑；本地终端会话仍可浏览本机文件。
- **监控**：左侧常驻服务器监控：CPU、内存、网络、磁盘、进程与主机信息；标题下方显示服务器 IP，点击复制并弹出「已复制」提示；可收起到侧边。
- **隧道**：本地转发、远程转发、SOCKS5。
- **批量命令**：在多台已保存主机上执行同一条命令。
- **界面**：深浅色主题、四套强调色、顶部多标签会话、可收起左侧监控栏/底部面板；主机密钥确认使用应用内弹窗。
- **设置**：主题、强调色、终端字号/字体/回滚缓冲、复制与查找行为；关闭设置时自动保存。

界面布局（类 FinalShell）：

```text
┌──────────────────────────────────────────────────────────────┐
│ 品牌 / 全局搜索                       主题  设置  窗口控制   │
├──────────────┬───────────────────────────────────────────────┤
│ 服务器监控    │ 标签栏：[1 测试] [2 本地终端] [3 新标签页] +  │
│              ├───────────────────────────────────────────────┤
│ CPU / 内存    │ 终端 / 远程编辑 / 快速连接                    │
│ 网络 / 磁盘   │                                               │
│ 进程 / 主机   │                                               │
│ 会话信息      ├───────────────────────────────────────────────┤
│              │ 文件 │ 传输 │ 隧道 │ 命令                      │
│              │ 本地文件                远端文件               │
├──────────────┴───────────────────────────────────────────────┤
│ 状态栏：连接状态 · CPU · 内存 · 网络 · 标签数 · UTF-8 · 版本 │
└──────────────────────────────────────────────────────────────┘
```

- 顶部标签栏左侧的文件夹按钮打开**连接管理器**，`+` 打开**快速连接**新标签页：页内列出已保存的服务器，点卡片立即新开一个「连接中」标签页，标签内以终端样式显示 TCP、加密协商、认证、打开终端等真实阶段与耗时；连接成功后该标签原地变成终端，快速连接页自动关闭。关闭「连接中」标签页可取消连接。
- 快速连接与连接管理器都支持搜索、分组、编辑/删除连接；连接失败时快速连接页保留，标签内直接显示错误，可重试或编辑连接。

## 环境要求

- Windows 10 1809+ / Windows 11，x64
- Go `1.27.3`
- PowerShell 7 或 Windows PowerShell 5.1
- 不需要单独安装 mygo：`go.mod` 已把 mygo CLI 固定为 Go tool

```powershell
go version    # 需要 1.27+
```

## 快速开始

开发运行：

```powershell
go tool mygo dev
```

## 打包命令

### 单文件免安装版（推荐）

```powershell
.\build-local.ps1
```

输出只有一个文件：

```text
build\windows-amd64\Xu.exe
```

这个 exe 已经包含：

- 应用图标（嵌入 PE 资源）
- `ghostty-vt.dll`（终端引擎）
- `mygo-sqlite3.dll`（会话数据库）

首次运行时，两个 DLL 会释放到用户缓存：

```text
%LOCALAPPDATA%\Xu\runtime\<构建哈希>\
```

之后启动直接复用，不需要安装，也不需要把 DLL 放在 exe 旁边。实现见 `internal/bundle/`：

- 构建脚本先跑一次普通 mygo build 取得 DLL
- 再把 DLL 放进 `internal/bundle/assets/`，用 `-tags=portable` 重新编译
- 程序入口在加载任何插件前释放 DLL，并设置 `MYGO_GHOSTTY_VT`、`MYGO_SQLITE_LIBRARY`

如果杀毒软件拦截 DLL 释放，应用会缺少终端或会话库功能；把项目目录/输出目录加入信任即可。

### 多文件构建

直接调用 mygo，不启用单文件封装：

```powershell
go tool mygo build -platform windows/amd64 -skip-installer
```

产物在 `build\windows-amd64\`，此时需要分发：

```text
Xu.exe
ghostty-vt.dll
mygo-sqlite3.dll
icon.png（可选）
```

`icon.png` 只作为构建资源；exe 自身已有图标，运行时不需要它。

### Windows 安装包

去掉 `-skip-installer`：

```powershell
go tool mygo build -platform windows/amd64
```

mygo 会生成 NSIS 安装包；需要准备 NSIS/可选签名环境。

### 图标没有刷新？

`mygo build` 生成的 exe 图标已经是新的。如果资源管理器/任务栏仍显示旧图标，通常是 Windows 图标缓存：

1. 先把 exe 复制到新目录或改名，确认图标是否变化
2. 取消旧的固定到任务栏/开始菜单，再重新固定
3. 清理图标缓存：

```powershell
taskkill /f /im explorer.exe
Remove-Item "$env:LOCALAPPDATA\Microsoft\Windows\Explorer\iconcache_*.db" -Force
Start-Process explorer.exe
```

程序内部标题栏左侧的 `Xu` 品牌标记是 UI 元素，不是 exe 图标；exe/资源管理器/任务栏图标使用嵌入的“小篆序”图标。

如果 PowerShell 阻止脚本执行：

```powershell
powershell -ExecutionPolicy Bypass -File .\build-local.ps1
```

## 测试

```powershell
.\test-local.ps1
```

等价于：

```powershell
go test ./...
go vet ./...
```

`test-local.ps1` 会把 `GOTMPDIR` 指向项目内 `.gotmp/`，减少 Windows 杀毒软件隔离 `*.test.exe` 的问题。

## 快捷键

| 操作 | 快捷键 |
| --- | --- |
| 新建连接 | `Ctrl N` |
| 本地终端 | `Ctrl T` |
| 新建快速连接标签页 | `Ctrl Alt T` |
| 连接管理器 | `Ctrl Alt C` |
| 文件面板 | `Ctrl Shift O` |
| 底部面板展开/收起 | `Ctrl J` |
| 服务器监控展开/收起 | `Ctrl B` / `Ctrl Alt B` |
| 终端内查找 | `Ctrl Shift F` |
| 全局搜索 | `Ctrl K` |
| 设置 | `Ctrl Alt S` |
| 服务器监控 | `Ctrl Shift M` |
| 批量命令 | `Ctrl Shift B` |
| 保存远程编辑 | `Ctrl S` |

## 数据与安全

默认数据目录：

```text
%USERPROFILE%\.xu\
├── xu.db             # SQLite：连接、分组、known_hosts、端口转发、设置
├── keys\             # OpenSSH 私钥：id_ed25519、id_ecdsa、id_rsa
└── tmp\              # 远程编辑临时文件
```

- 密码通过 `mygo.Secrets` 保存到系统凭据存储，不写入 SQLite 明文。
- 私钥放在 `%USERPROFILE%\.xu\keys\`，当前版本不支持带口令的私钥。
- 首次连接会记录主机密钥指纹；指纹变化会阻断连接。
- 会话数据只记录连接元信息，不保存终端输出。
- 设置中的主题/强调色即时生效，其余设置在关闭设置弹窗时自动保存。

## 项目结构

```text
main.go                    应用入口、窗口、数据目录
internal/app/              UI、会话、文件、监控、转发、批量命令、设置
internal/sshx/             SSH 拨号与认证
internal/monitorx/         Linux 指标采集
internal/tunnelx/          本地/远程/SOCKS5 转发
internal/store/            SQLite 会话、分组、known_hosts、设置
internal/vault/            系统凭据存储封装
internal/bundle/           单文件版 DLL 嵌入与首次运行释放
build-local.ps1            单文件免安装包构建脚本
test-local.ps1             测试 + vet
resources/icon.png         小篆「序」应用图标
mygo.json                  应用名称、版本、输出目录
```


## 注意事项

- 这是实验性项目，mygo 仍处于 `0.x`，升级依赖时请先锁定版本并回归测试。
- 当前构建未签名，Windows SmartScreen/杀毒软件可能提示风险；开发时可把项目目录加入杀毒软件信任。
- 终端使用 Ghostty VT 原生库；单文件构建会把 `ghostty-vt.dll` 嵌入 exe，首次运行释放到 `%LOCALAPPDATA%\Xu\runtime\`，不需要手动复制。
- 删除 `%LOCALAPPDATA%\Xu\runtime\` 不会影响会话数据；下次启动会用嵌入的 DLL 重新释放。
- 远端路径使用 `/`，本地路径使用 Windows 路径规则。
