# ECorpLink

ECorpLink 是一个面向 CorpLink 的第三方 VPN 客户端，当前以 macOS DMG 为主要交付形态，
提供基于 Wails v3 的图形界面、后台 daemon、分流规则、Fake IP DNS，以及嵌入式
WireGuard userspace 设备。

## 当前形态

- 主入口是 macOS GUI 应用，当前正式交付产物是 DMG。
- GUI 会安装并管理内部 daemon helper。
- daemon helper 是内部组件，不作为用户单独使用的 CLI 发布。
- Linux / Windows 的应用层支持会尽量保留并保持可编译，但目前没有进行过真实环境测试。
- WireGuard userspace 实现来自官方 `wireguard-go`，以源码嵌入本仓库。

## 路径

- 配置文件：`~/.ecorplink/config.json`
- PID 文件：`~/.ecorplink/ecorplink.pid`
- 日志文件：`~/.ecorplink/ecorplink.log`
- 内部 daemon helper：`~/.ecorplink/bin/ecorplink-daemon`

## 构建

构建入口：

```bash
./scripts/build_wails.sh
```

版本号不在本地文件中维护。发布构建由 Git tag 注入版本号，例如 `v1.0.0`；
本地构建可手动传入 `VERSION`，不传时使用 `dev`。

```bash
VERSION=1.0.0 ./scripts/build_wails.sh
```

常用验证构建：

```bash
./scripts/build_wails.sh --skip-tests
```

构建过程会临时生成 app bundle 或 GUI binary，但打包完成后只保留压缩后的发布产物。

## 开发

运行测试：

```bash
go test ./...
```

Wails 相关构建任务在 `Taskfile.yml` 中维护。`scripts/build_wails.sh` 是打包主入口。

Linux / Windows daemon helper 可做交叉编译检查：

```bash
task daemon:linux
task daemon:windows
```

平台状态：

- macOS：主要支持目标，DMG 构建已验证。
- Windows：应用层代码保留，当前做编译层维护，尚未在真实 Windows 环境验证 TUN、路由、
  DNS 和服务安装流程。
- Linux：daemon 和应用核心包做编译层维护，尚未在真实 Linux 环境验证 TUN、路由、DNS
  和服务安装流程。Linux GUI 依赖 Wails/Linux 本机图形与 cgo 环境，需在 Linux 环境中单独验证。

## WireGuard

`wireguard-go/` 基于官方上游：

```text
https://github.com/WireGuard/wireguard-go
```

该目录保留上游 `LICENSE`。本项目只保留 ECorpLink 当前程序需要的本地改动。

## 致谢

感谢 [Wails v3](https://v3.wails.io/) 提供跨平台 Go + Web 图形界面框架。

感谢 [PinkD/corplink-rs](https://github.com/PinkD/corplink-rs) 项目提供的 CorpLink
协议和实现参考。

## License

本项目根目录代码使用 MIT License，见 [LICENSE](LICENSE)。

`wireguard-go/` 目录遵循其自身保留的上游许可证。

Windows 使用的预编译 Wintun DLL 按架构保存在
`internal/tun/wintun/bin/{amd64,x86,arm64,arm}/wintun.dll`，构建时会按 `GOARCH`
嵌入对应版本。这些 DLL 遵循 WireGuard LLC 的 Wintun Prebuilt Binaries License，见
[internal/tun/WINTUN-PREBUILT-BINARIES-LICENSE.txt](internal/tun/WINTUN-PREBUILT-BINARIES-LICENSE.txt)。
