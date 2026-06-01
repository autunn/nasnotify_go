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
├── internal/wechatgateway/     # 内置微信网关状态、二维码、轮询服务
├── frontend/ugreen-app/        # Vite 黑金后台页面
├── macos/NasNotifyGo/          # macOS 桌面窗口壳应用
├── packaging/ugreen-native-app/# 绿联 UPK 打包骨架
├── scripts/                    # macOS / UPK 构建脚本
├── tools/ugcli/                # 本地放置 ugcli，默认不提交二进制
├── Dockerfile
└── go.mod
```

运行时会使用当前工作目录下的 `config/` 和 `data/` 保存配置、登录态与通知记录。

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

### 微信 ClawBot

后台的“微信绑定”页可以生成二维码并扫码登录。绑定完成后，微信里可发送：

```text
菜单
查询菜单
控制菜单
状态
通知
存储
Docker
进程
备份
电源
UPS
测试
风扇2
CPU1
```

查询类命令会优先返回黑金图片卡片。

## macOS 构建

本仓库用于上传 GitHub 后构建 macOS 程序。GitHub Actions 会自动执行：

1. 构建 Go 后端。
2. 执行 `frontend/ugreen-app` 的 Vite 构建。
3. 构建 Swift 桌面窗口壳应用。
4. 将后端和前端 `dist` 放入 app bundle 的 `Resources`。
5. 生成 DMG。

本地 macOS 构建：

```bash
./scripts/build-macos-app.sh
```

Windows 环境不能完整验证 Swift/macOS app 打包，但可以验证 Go 测试和 Vite 构建。

## Docker

```bash
docker build -t nasnotify-go .
docker run --rm -p 5080:5080 \
  -v "$(pwd)/config:/app/config" \
  -v "$(pwd)/data:/app/data" \
  nasnotify-go
```

Dockerfile 会先构建 Vite 前端，再把 `dist` 复制到最终镜像的 `/app/www`。

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
```

`node_modules/`、`frontend/ugreen-app/dist/`、`build/`、`dist/`、UPK 打包输出和 `ugcli` 二进制均不提交到仓库。

## 参考

- [bilibili-koryking/nasnotify](https://github.com/bilibili-koryking/nasnotify)
- [xbclub/ugreen-monitor](https://github.com/xbclub/ugreen-monitor)
