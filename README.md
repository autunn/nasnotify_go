# NasNotify-Go

NasNotify-Go 是一个面向绿联 NAS 的通知与控制中心。当前版本已迁移为 **Go API + Vite 前端** 架构，保留企业微信通道，同时新增微信 ClawBot 网关，并同步黑金风格的网页后台和图片消息卡片。

## 主要能力

- **绿联 NAS 查询与控制**：状态、通知、存储、Docker、进程、备份、电源、UPS、测试通知，以及风扇/CPU 模式控制。
- **图片卡片优先**：常用查询命令优先渲染为黑金风格 PNG 卡片，发送失败时回退为文本或企业微信图文消息。
- **双微信通道**：保留企业微信配置与回调菜单，新增微信 ClawBot 扫码绑定与消息轮询。
- **温度兼容解析**：CPU 温度会兼容 `cpuTemp`、`temperature`、`cpu_temp`、`cpu_temperature` 等字段别名。
- **跨平台构建**：Go 后端可直接运行；同一仓库可以分别产出 macOS DMG、Docker 镜像和绿联 UPK。

## 项目结构

```text
nasnotify_go/
├── cmd/nasnotify/              # Go 程序入口
├── internal/app/               # HTTP API、静态资源、任务调度、初始化流程
├── internal/api/               # 企业微信回调与通用 Webhook
├── internal/config/            # 配置读写、敏感字段保留、默认值
├── internal/nas/               # 绿联 NAS API、查询命令、消息文本与图片卡片数据
├── internal/notify/            # 企业微信、ClawBot、统一推送入口
├── internal/notifycard/        # 黑金 PNG 卡片渲染器与字体资源
├── internal/wechatgateway/     # 内置 ClawBot 网关状态、二维码、轮询服务
├── frontend/ugreen-app/        # Vite 黑金后台页面
├── macos/NasNotifyGo/          # macOS 桌面窗口壳应用
├── packaging/ugreen-native-app/# 绿联 UPK 打包骨架
├── scripts/                    # macOS / UPK 构建脚本
├── tools/ugcli/                # 本地放置 ugcli，默认不提交二进制
├── Dockerfile
└── go.mod
```

运行时会使用 `data/` 保存配置、登录态与 ClawBot 状态，使用 `log/` 保存运行日志。

## 本地运行

```bash
go mod download
go run ./cmd/nasnotify
```

默认监听端口为 `5080`，启动后访问：

```text
http://127.0.0.1:5080
```

首次进入后台需要完成初始化。默认管理密码只用于兼容旧配置，正式使用前请在后台保存新的管理员密码。

## 前端开发

```bash
cd frontend/ugreen-app
npm ci
npm run dev
```

生产构建：

```bash
cd frontend/ugreen-app
npm ci
npm run build
```

Go 服务会优先读取 macOS app/Docker 资源目录中的 `www`，开发环境也会兼容本仓库的 `frontend/ugreen-app/dist`。

## 配置通道

### 绿联 NAS 地址

后台“基础设置”里需要填写：

- `本机 NAS 显示名称`
- `NAS 地址 / IP`
- `NAS MAC 地址`
- `本机 NAS 端口`
- `本机 NAS 管理账号`
- `本机 NAS 管理密码`

UPK 安装在绿联 NAS 内部时，`NAS 地址 / IP` 可以保持默认 `127.0.0.1`。Docker 和 macOS 版本运行在 NAS 外部时，需要改为真实设备 IP 或可解析域名，例如 `192.168.1.9`。如需使用远程唤醒，必须填写这台绿联 NAS 的 MAC 地址。

### 企业微信

企业微信配置会继续保留：

- `CorpID`
- `AgentID`
- `CorpSecret`
- `Token`
- `EncodingAESKey`
- `NAS 跳转地址`
- `图文封面 API`
- `企业微信代理地址`

回调入口为：

```text
http://<你的公网地址>:5080/wx-receive
```

保存配置后，程序会尝试同步企业微信自定义菜单。企业微信图片消息会先上传 PNG，再发送图片；失败时回退到文本/图文。
企业微信菜单按 `查询`、`服务`、`控制` 三组组织：查询负责巡检、状态、通知和存储；服务负责 Docker、进程、备份、电源和 UPS；控制提供风扇静音/标准/全速、CPU 性能说明和远程唤醒。CPU 三档也可直接发送 `CPU0`、`CPU1`、`CPU2` 执行。

### 微信 ClawBot

后台的“微信 ClawBot”页可以生成二维码并扫码登录。绑定完成后，微信里可发送：

```text
菜单
查询菜单
控制菜单
巡检
状态
通知
存储
Docker
进程
备份
电源
UPS
测试
唤醒
风扇2
CPU1
```

查询类命令会优先返回黑金图片卡片；`唤醒` / `wol` 会向后台配置的本机绿联 NAS 发送 WOL 魔术包。

## macOS 构建

macOS 产物使用 `scripts/build-macos-app.sh` 统一构建。脚本会构建 Go 后端、Vite 前端、Swift 桌面窗口壳应用，并生成未公证的自用 DMG。没有 Apple Developer 账号时，脚本默认会做 ad-hoc 本地签名，保证 app bundle 内部代码签名结构完整。

本地构建：

```bash
./scripts/build-macos-app.sh
```

GitHub Actions 的 `Build macOS DMG` 可以手动触发。默认 `sign_and_notarize=false`，不需要 Apple 付费开发者账号，也不需要配置证书 secrets，会生成自用的未公证 DMG。

未公证 DMG 首次安装后，macOS 可能会提示“已损坏，无法打开”。自用时先把应用拖到 `/Applications`，再在 Mac 上执行一次：

```bash
xattr -dr com.apple.quarantine "/Applications/NasNotify-Go.app"
```

如果需要把 DMG 发给其他用户正常下载安装，需要 Apple Developer Program 账号，并在 GitHub 仓库 Settings -> Secrets and variables -> Actions 中配置：

```text
MACOS_CERTIFICATE_P12_BASE64
MACOS_CERTIFICATE_PASSWORD
MACOS_CODESIGN_IDENTITY
APPLE_ID
APPLE_TEAM_ID
APPLE_APP_SPECIFIC_PASSWORD
```

其中 `MACOS_CERTIFICATE_P12_BASE64` 是 Developer ID Application 证书 `.p12` 文件的 base64 内容。配置完成后手动触发 `Build macOS DMG`，勾选 `sign_and_notarize=true`，Actions 会导入证书、签名 app、提交 Apple 公证、staple 公证票据并签名/公证 DMG。

本地 macOS 也可以直接使用同一套脚本正式签名公证：

```bash
export MACOS_CODESIGN_IDENTITY="Developer ID Application: Your Name (TEAMID)"
export APPLE_NOTARY_KEYCHAIN_PROFILE="nasnotify-notary"
SIGN_AND_NOTARIZE=1 ./scripts/build-macos-app.sh
```

Windows 环境不能完整验证 Swift/macOS app 打包，但可以验证 Go 测试和 Vite 构建。

## Docker

推荐使用 Docker Compose。先复制环境变量模板：

```bash
cp .env.example .env
```

普通 bridge 网络模式：

```bash
docker compose up -d nasnotify
```

启动后访问：

```text
http://127.0.0.1:5080
```

如果 Docker 运行在 NAS 所在局域网内，并且需要更稳定地访问内网 NAS 或使用远程唤醒，可以改用 host 网络模式：

```bash
docker compose --profile hostnet up -d nasnotify-hostnet
```

注意不要同时启动 `nasnotify` 和 `nasnotify-hostnet`，否则会争用同一个监听端口。

配置、登录态和 ClawBot 状态会保存到：

```text
./data
```

运行日志会保存到：

```text
./log
```

Docker 镜像会先构建 Vite 前端，再把 `dist` 复制到最终镜像的 `/app/www`。

如需验证 Compose 配置：

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\check-docker-compose.ps1
```

如需本地构建镜像并用本地镜像启动：

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\check-docker-compose.ps1 -Build -SmokeTest
```

本地构建会叠加 `docker-compose.local.yml`，把镜像标签改为 `nasnotify-go:local`，不会影响默认使用的 `ghcr.io/autunn/nasnotify-go:latest`。

也可以直接使用 Docker 命令手动构建：

```bash
docker build -t nasnotify-go .
docker run --rm -p 5080:5080 \
  -e UGAPP_DATA_DIR=/app/data \
  -e UGAPP_LOG_DIR=/app/log \
  -e UGAPP_WEB_DIR=/app/www \
  -v "$(pwd)/data:/app/data" \
  -v "$(pwd)/log:/app/log" \
  nasnotify-go
```

## 绿联 UPK 构建

UPK 和 DMG、Docker 共用同一套 Go API、Vite 前端和图片卡片逻辑，但产物独立输出。

本地 Windows 构建：

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\build-ugreen-native-app.ps1 -Build 1 -Arch all
```

构建流程会执行：

1. 安装/复用前端依赖并执行 Vite 构建。
2. 将 `frontend/ugreen-app/dist` 同步到 `packaging/ugreen-native-app/rootfs_common/www`。
3. 交叉编译 Linux `amd64`、`arm64` 后端。
4. 调用 `ugcli check` 和 `ugcli pack` 生成 UPK。

默认 `ugcli` 路径为：

```text
tools/ugcli/ugcli-v1.1.0.12-windows-amd64.exe
```

该二进制默认不提交到 Git。也可以通过 `-UgcliPath` 指定本机其他路径。

生成文件位于：

```text
packaging/ugreen-native-app/build_dir/pkgs/upk/
```

UPK 访问模式保持官方应用式 route 打开：NAS 门户、DDNS、UGLink 都走主入口路由和 Unix socket 代理，不需要给用户额外暴露端口。

## 验证命令

```bash
go test ./...
node --check frontend/ugreen-app/src/main.js
npm --prefix frontend/ugreen-app ci
npm --prefix frontend/ugreen-app run build
node scripts/build-ugreen-frontend.mjs
powershell -ExecutionPolicy Bypass -File .\scripts\check-docker-compose.ps1 -Build -SmokeTest
```

`node_modules/`、`frontend/ugreen-app/dist/`、`build/`、`dist/`、UPK 打包输出和 `ugcli` 二进制均不提交到仓库。

## 参考

- [bilibili-koryking/nasnotify](https://github.com/bilibili-koryking/nasnotify)
- [xbclub/ugreen-monitor](https://github.com/xbclub/ugreen-monitor)
