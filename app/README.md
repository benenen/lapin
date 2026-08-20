# Lapin 移动端（Flutter）

Android / iOS 双平台客户端，对接 Lapin 现有的 Go API。设计依据是
[`docs/flutter-app-plan.md`](../docs/flutter-app-plan.md)，本文只记录跑起来需要知道的事。

当前范围是最小可用闭环：**登录 → 课程列表 → 章节正文（Markdown 渲染）**。
白板尚未实现，见文末 TODO。标注已按 `quote` 锚定实现，见 `lib/features/annotations/` 与 `docs/superpowers/specs/2026-08-20-flutter-quote-anchored-annotations-design.md`。

## 跑起来

先起后端（仓库根目录）：

```sh
bash scripts/stop_watch.sh   # 先停掉可能残留的实例
make watch                   # 监听 127.0.0.1:8080
```

再跑 app：

```sh
export PATH=/root/workspace/flutter/bin:$PATH
cd app
flutter pub get
flutter run -d <device> --dart-define=API_BASE_URL=http://10.0.2.2:8080
```

`API_BASE_URL` 必须按运行环境给：

| 运行环境 | 地址 |
| --- | --- |
| Android 模拟器 | `http://10.0.2.2:8080`（模拟器里 `10.0.2.2` 指向宿主机 loopback，即 `make watch` 所在处） |
| Android 真机 | 宿主机局域网 IP，且后端要监听 `0.0.0.0` |
| iOS 模拟器 | `http://localhost:8080` |

不传则用默认值 `http://10.0.2.2:8080`。

## 验证

```sh
flutter analyze
flutter test                                   # 纯 Dart 单测，不需要设备
flutter test integration_test/reading_flow_test.dart -d emulator-5554 \
  --dart-define=API_BASE_URL=http://10.0.2.2:8080   # 端到端，需要设备 + 后端
flutter build apk --debug                      # Android 构建
```

端到端测试默认用 `admin@localhost` / `admin12345678` 登录，可用
`--dart-define=E2E_EMAIL=... --dart-define=E2E_PASSWORD=...` 覆盖。

## 目录结构

```
lib/
├── main.dart                 # 构建 ApiClient 并注入 ProviderScope
├── app.dart                  # MaterialApp.router + 主题
├── core/
│   ├── config/env.dart       # --dart-define 读取的构建期配置
│   ├── network/
│   │   ├── api_client.dart       # dio 实例、cookie 罐、{data}/{error} 封套拆解
│   │   ├── csrf_interceptor.dart # 写请求回填 X-CSRF-Token
│   │   └── api_exception.dart    # 把服务端错误码转成可判定的异常
│   └── router/app_router.dart    # go_router + 登录守卫
└── features/
    ├── auth/{data,domain,application,presentation}
    ├── subjects/{data,domain,presentation}
    └── chapter/presentation
```

约定：`features/*` 之间不互相 import，要复用就下沉到 `core/` 或 `shared/`。

## 认证：为什么是 Cookie 而不是 Bearer

Lapin 的 `/api/v1/*` 全部走会话认证（`RequireSession`），写操作还要过
`RequireCSRF`；Bearer 访问令牌只在 `/openapi/v1/*` 上被接受，而那组没有任何
读取接口。所以移动端只能带 cookie 罐。

具体机制：

1. `POST /api/v1/auth/login` 返回 `lapin_session`（HttpOnly）和 `lapin_csrf` 两个 cookie，有效期 7 天
2. `CookieManager` 把它们存进 `PersistCookieJar`（落盘，重启 app 仍在）
3. 每个非 GET 请求，`CsrfInterceptor` 从罐里取 `lapin_csrf` 填进 `X-CSRF-Token` 头

原生 app 不发 `Origin` 头，而服务端的来源校验在 `Origin` 为空时放行，所以登录
不需要服务端做任何改动。

**已知短板**：会话 7 天且没有刷新接口，用户每周要重新登录一次。改进需要服务端
配合，见 `docs/flutter-app-plan.md` §6.1。

## iOS

代码是双平台共享的，`ios/` 工程也已生成，但**本仓库的开发环境是 Linux，无法构建
iOS**：`flutter build ipa`、Xcode 签名、CocoaPods、TestFlight 上传都要求 macOS。

因此本次未验证 iOS 端。要补齐需要一台 Mac 或 CI 的 macOS runner，步骤见
`docs/flutter-app-plan.md` §5 与 §7。iOS 相关的配置（Info.plist 权限文案、签名、
Associated Domains）在那份文档里已经写好，届时照做即可。

## TODO

- **白板**：透明手写层。Flutter 侧比 Web 简单（无画布尺寸上限），但必须产出服务端
  `validWhiteboardData` 认可的 Excalidraw 文档格式，约束见 §6.5。
- **讨论**：章节评论的读写。
- **离线缓存**：按方案接 Drift，缓存章节正文。
- **会话续期**：等服务端提供刷新机制后接入。
