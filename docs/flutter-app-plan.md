# Lapin 移动端：Flutter 双平台实施方案

日期：2026-08-19
状态：调研方案，未落地任何代码

本文回答两个问题：用 Flutter 做一个同时跑 Android 和 iOS 的 app 具体怎么做；以及把 Lapin 做成 app 时，与现有 Go 后端如何对接。

文中「已核实」标记的结论来自本仓库源码或本机实测，其余为通用工程实践。

---

## 0. 先看结论

| 判断 | 依据 |
| --- | --- |
| Flutter 单代码库产出双平台，技术上没有障碍 | 本机已装 Flutter 3.44.9 stable / Dart 3.12.2（`/root/workspace/flutter`） |
| **现有 API 可以直接对接，不必改服务端** —— 但要用 Cookie + CSRF，不是 Bearer | 已核实，见 §6.1 |
| 阻塞项：iOS 构建必须有 macOS + Xcode | 本机是 Linux，`flutter build ipa` 跑不了，只能靠 CI 的 macOS runner 或本地 Mac |
| 标注功能不能照搬 Web 的字符偏移 | 已实测，见 §6.4 |
| 白板功能在 Flutter 上比 Web 简单，但数据格式被服务端校验锁死 | 已核实，见 §6.5 |

---

## 1. 技术选型与架构

### 1.1 Flutter 的取舍

**适合 Lapin 的理由**：一套 Dart 代码产出双平台原生包；自绘渲染保证两端 UI 完全一致（Lapin 的章节排版和标注高亮对一致性敏感）；`CustomPaint` 天然适合白板；热重载让 UI 迭代很快。

**代价**：包体积起步就比原生大（Android release APK 约 15–20 MB，iOS 约 25–35 MB）；Dart 生态在某些垂直领域（富文本编辑、PDF）弱于 JS；升级 Flutter 大版本偶尔要跟着改代码；**iOS 打包必须有 Mac**，这是硬约束，无解。

**什么时候不该选**：如果只是想把现有 Vue 页面装进壳里，用 WebView 套壳（或 PWA）成本低一个数量级。选 Flutter 的前提是要做原生交互——对 Lapin 来说就是白板手写、离线阅读、系统级分享。

### 1.2 单代码库如何产出双平台

`flutter create` 生成的工程里，`lib/` 是共享的 Dart 代码（99% 的工作量），`android/` 和 `ios/` 是两个真实的原生工程（Gradle 工程和 Xcode 工程），只在需要平台能力时才动。

```
flutter build apk --release          # Android，Linux/macOS/Windows 都能跑
flutter build appbundle --release    # Google Play 要的 AAB
flutter build ipa --release          # iOS，只能在 macOS 上跑
```

平台差异在 Dart 层用 `Platform.isIOS` / `Theme.of(context).platform` 分支，或用 `.adaptive` 系列组件（`Switch.adaptive` 等）；需要调原生 API 时走 `MethodChannel` 或现成插件。

### 1.3 状态管理选型

| 方案 | 特点 | 结论 |
| --- | --- | --- |
| **Riverpod** | 编译期安全、不依赖 BuildContext、测试无需 pump widget、`AsyncValue` 原生表达 loading/error/data | **推荐** |
| Bloc | 事件流建模严谨、适合复杂状态机、样板代码多 | 团队已有 Bloc 经验时选 |
| Provider | 简单、官方早期方案、大型项目容易变成上帝对象 | 不建议新项目用 |

选 Riverpod 的具体理由：Lapin 的每个页面本质是「拉一份远端数据 + 一份本地草稿」，`AsyncNotifierProvider` 正好对上；且 `ProviderContainer` 可以在纯 Dart 测试里跑，不需要 widget test，测试成本低。

```yaml
# pubspec.yaml 片段
dependencies:
  flutter_riverpod: ^2.6.1
  riverpod_annotation: ^2.6.1
dev_dependencies:
  riverpod_generator: ^2.6.3
  build_runner: ^2.4.13
```

### 1.4 路由

用 **go_router**：声明式、支持深链接（Lapin 的 `/subjects/:id` 可以直接映射成 app 内路由和外部 URL）、有 `redirect` 钩子做登录守卫。

```dart
final router = GoRouter(
  redirect: (context, state) {
    final signedIn = ref.read(sessionProvider).valueOrNull != null;
    if (!signedIn && state.matchedLocation != '/sign-in') return '/sign-in';
    return null;
  },
  routes: [
    GoRoute(path: '/sign-in', builder: (_, __) => const SignInPage()),
    GoRoute(path: '/subjects/:id', builder: (_, s) => SubjectPage(id: s.pathParameters['id']!)),
  ],
);
```

深链接需要在两端各配一次：Android 的 `intent-filter`（§4.2）和 iOS 的 Associated Domains（§5.2）。

### 1.5 网络层

用 **dio** + **cookie_jar**。选 dio 而不是 `package:http` 的唯一硬理由：Lapin 的写接口需要 Cookie 和 CSRF 头联动（§6.1），dio 的拦截器机制是最省事的落点。

```yaml
dependencies:
  dio: ^5.7.0
  cookie_jar: ^4.0.8
  dio_cookie_manager: ^3.1.1
  path_provider: ^2.1.5   # 持久化 cookie 到磁盘
```

### 1.6 本地缓存

| 用途 | 选型 |
| --- | --- |
| 会话 cookie | `PersistCookieJar`（落盘到 `getApplicationSupportDirectory()`） |
| 章节正文、标注、白板（离线阅读） | **Drift**（SQLite，类型安全、支持迁移、可写测试） |
| 轻量偏好（主题、上次阅读位置） | `shared_preferences` |
| 敏感数据（如果将来改用 token 认证） | `flutter_secure_storage`（Android Keystore / iOS Keychain） |

不建议用 Hive：其 2.x 已停止维护，3.x 生态未稳定。

---

## 2. 项目骨架

### 2.1 起项目

```sh
flutter create \
  --org com.lapin \
  --project-name lapin_app \
  --platforms=android,ios \
  --description "Lapin 学习平台移动端" \
  lapin_app
cd lapin_app
flutter pub add flutter_riverpod go_router dio cookie_jar dio_cookie_manager \
  path_provider drift shared_preferences
flutter pub add --dev build_runner riverpod_generator drift_dev custom_lint riverpod_lint
```

`--org` 决定 applicationId / bundle identifier（`com.lapin.lapin_app`），**上架后无法更改**，起名时就要定死。

### 2.2 目录结构（features / core / shared 分层）

```
lib/
├── main.dart                    # 仅做 runApp + ProviderScope
├── app.dart                     # MaterialApp.router、主题、本地化
├── core/                        # 与业务无关的基础设施
│   ├── network/
│   │   ├── dio_client.dart      # dio 实例、BaseOptions、拦截器装配
│   │   ├── csrf_interceptor.dart
│   │   └── api_exception.dart   # 把 {error:{code,message}} 映射成异常
│   ├── storage/
│   │   ├── database.dart        # Drift
│   │   └── cookie_store.dart
│   ├── router/app_router.dart
│   └── config/env.dart          # --dart-define 读取 baseUrl 等
├── features/                    # 按业务垂直切分，每个 feature 自包含
│   ├── auth/
│   │   ├── data/auth_repository.dart
│   │   ├── domain/session.dart
│   │   └── presentation/sign_in_page.dart
│   ├── subjects/
│   ├── chapter/                 # 正文阅读、标注标记
│   ├── annotations/
│   └── whiteboard/
└── shared/                      # 跨 feature 复用的 UI 与工具
    ├── widgets/
    └── extensions/
```

分层原则：`features/*` 之间**不允许互相 import**，要复用就下沉到 `shared/` 或 `core/`。这条用 `custom_lint` 配合 `import_lint` 规则可以自动检查，避免日久变成一团。

每个 feature 内部 `data / domain / presentation` 三层：`data` 负责 API 与本地库，`domain` 是纯 Dart 模型和业务规则（**可以脱离 Flutter 单测**），`presentation` 是 widget 和 Riverpod provider。

### 2.3 平台目录职责

**`android/`** —— 一个完整 Gradle 工程：

| 文件 | 用途 |
| --- | --- |
| `app/build.gradle.kts` | SDK 版本、签名配置、构建变体 |
| `app/src/main/AndroidManifest.xml` | 权限、Activity、intent-filter、application 属性 |
| `app/src/main/res/` | 图标、启动图、strings |
| `key.properties`（不入库） | keystore 路径与口令 |
| `app/proguard-rules.pro` | R8 保留规则 |

**`ios/`** —— 一个完整 Xcode 工程：

| 文件 | 用途 |
| --- | --- |
| `Runner.xcodeproj` / `Runner.xcworkspace` | Xcode 工程（装了 pod 之后用 workspace 打开） |
| `Runner/Info.plist` | Bundle 信息、权限文案、URL scheme |
| `Podfile` | CocoaPods 依赖（Flutter 插件的原生部分） |
| `Runner/Assets.xcassets` | 图标、启动图 |
| `Runner.entitlements` | 推送、Associated Domains 等能力开关 |

日常开发几乎不碰这两个目录；碰它们的时机只有：加权限、改图标、配签名、接原生 SDK。

---

## 3. 环境与工具链

### 3.1 Flutter SDK 版本管理（fvm）

团队和 CI 必须锁同一个 Flutter 版本，否则会出现「我这儿能编你那儿报错」。用 **fvm**：

```sh
dart pub global activate fvm
fvm install 3.44.9
fvm use 3.44.9          # 生成 .fvmrc 与 .fvm/，把 .fvmrc 提交进仓库
fvm flutter --version   # 之后所有命令加 fvm 前缀
```

`.gitignore` 里加 `.fvm/flutter_sdk`（那是软链），但 **`.fvmrc` 必须提交**。

本机现状（已核实）：`/root/workspace/flutter` 是 Flutter 3.44.9 stable 的 SDK checkout，`flutter` 与 `dart` 在 `bin/` 下，可直接 `export PATH=/root/workspace/flutter/bin:$PATH` 使用。

### 3.2 Android 工具链

```sh
# JDK 17（AGP 8.x 要求；21 也可，但 17 是当前最稳的组合）
sudo apt install openjdk-17-jdk
# Android SDK：装 Android Studio，或只装 command line tools
sdkmanager "platform-tools" "platforms;android-35" "build-tools;35.0.0"
sdkmanager --licenses            # 必须全部 accept，否则构建失败
flutter doctor --android-licenses
```

Gradle 不用手动装，工程自带 wrapper。首次构建会下载 Gradle 发行版和依赖，国内网络建议配 `~/.gradle/init.gradle.kts` 走镜像。

### 3.3 iOS 工具链（必须 macOS）

```sh
xcode-select --install
sudo xcodebuild -license accept
sudo gem install cocoapods        # 或 brew install cocoapods
cd ios && pod install             # 每次增删插件后都要跑
```

**这是本方案唯一的硬阻塞**：本机是 Linux，无法构建、无法签名、无法上传 App Store。可选解法：(a) 一台 Mac 或 Mac mini；(b) GitHub Actions 的 `macos-latest` runner（公开仓库免费，私有仓库按分钟计费且 macOS 单价是 Linux 的 10 倍）；(c) Codemagic / Bitrise 这类专做移动 CI 的托管服务。

### 3.4 开发期：模拟器与热重载

```sh
flutter devices                          # 列出可用设备
flutter emulators --launch <emulator_id> # Android 模拟器
open -a Simulator                        # iOS 模拟器（仅 macOS）
flutter run --dart-define=API_BASE_URL=http://10.0.2.2:8080
```

`r` 热重载（保留状态），`R` 热重启（丢状态），`q` 退出。

**联调 Lapin 后端的地址坑**：Android 模拟器里 `localhost` 指模拟器自己，宿主机要用 `10.0.2.2`；iOS 模拟器可以直接用 `localhost`；真机则要用局域网 IP。用 `--dart-define` 传，不要写死。

Lapin 后端默认监听 `127.0.0.1:8080`（`make watch`），真机联调需要改成监听 `0.0.0.0`。另外 Android 9+ 默认禁止明文 HTTP，开发期需要 `android:usesCleartextTraffic="true"`（仅 debug 变体）或配 `network_security_config.xml`。

---

## 4. Android 要点

### 4.1 SDK 版本与 Gradle

`android/app/build.gradle.kts`：

```kotlin
android {
    namespace = "com.lapin.lapin_app"
    compileSdk = 35            // 跟随最新稳定版
    ndkVersion = flutter.ndkVersion

    defaultConfig {
        applicationId = "com.lapin.lapin_app"   // 上架后不可改
        minSdk = 24            // Android 7.0，覆盖率 >98%，低于此 Flutter 支持吃力
        targetSdk = 35         // Google Play 强制要求：新应用需在最新版发布后一年内跟进
        versionCode = flutter.versionCode
        versionName = flutter.versionName
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
        isCoreLibraryDesugaringEnabled = true   // 某些插件需要
    }
}
```

`versionCode` 每次上传 Play 必须严格递增，`versionName` 是给人看的。二者由 `pubspec.yaml` 的 `version: 1.0.0+1` 驱动（`+` 后面是 versionCode）。

### 4.2 权限与 Manifest

只声明真正用到的权限——Play 审核会追问用途：

```xml
<manifest>
  <uses-permission android:name="android.permission.INTERNET"/>
  <!-- 仅当要下载章节图片到相册时才需要，Android 13+ 用细分权限 -->
  <uses-permission android:name="android.permission.READ_MEDIA_IMAGES"/>

  <application android:label="Lapin" android:icon="@mipmap/ic_launcher">
    <activity android:name=".MainActivity" android:exported="true">
      <!-- 深链接：https://lapin.example.com/subjects/xxx 直接打开 app -->
      <intent-filter android:autoVerify="true">
        <action android:name="android.intent.action.VIEW"/>
        <category android:name="android.intent.category.DEFAULT"/>
        <category android:name="android.intent.category.BROWSABLE"/>
        <data android:scheme="https" android:host="lapin.example.com"/>
      </intent-filter>
    </activity>
  </application>
</manifest>
```

`autoVerify` 需要服务端在 `https://lapin.example.com/.well-known/assetlinks.json` 提供指纹文件才能免选择器直达。

### 4.3 R8 / ProGuard

Flutter release 默认开启 R8 收缩。绝大多数情况不需要自定义规则；用到反射的原生 SDK（如某些推送）才需要在 `proguard-rules.pro` 里 keep。若崩溃栈全是混淆符号，构建时加 `--obfuscate --split-debug-info=build/symbols` 并把符号文件存档，用 `flutter symbolize` 还原。

### 4.4 签名

```sh
keytool -genkey -v -keystore ~/lapin-upload.jks \
  -keyalg RSA -keysize 2048 -validity 10000 -alias upload
```

`android/key.properties`（**不入库**，加进 `.gitignore`）：

```properties
storePassword=***
keyPassword=***
keyAlias=upload
storeFile=/absolute/path/lapin-upload.jks
```

`build.gradle.kts` 里读取它并配 `signingConfigs.release`。**keystore 丢失 = 无法再更新已上架应用**（除非启用了 Play App Signing 并申请重置），必须离线备份。

### 4.5 上架 Google Play

1. 注册开发者账号（一次性 25 美元）
2. Play Console 创建应用，填分级问卷、隐私政策 URL、数据安全表单
3. `flutter build appbundle --release` 产出 `build/app/outputs/bundle/release/app-release.aab`
4. 先传到内部测试轨道验证，再逐步放量到正式轨道
5. 首次审核通常 1–7 天；新开发者账号还需满足 12 名测试者连续 14 天的封闭测试要求（个人开发者政策）

---

## 5. iOS 要点

### 5.1 Info.plist 与权限

iOS 的权限文案是**强制**的：用到某能力却没写用途说明，运行时直接崩溃，且审核必拒。

```xml
<key>NSPhotoLibraryAddUsageDescription</key>
<string>保存章节插图到你的相册</string>
<key>NSCameraUsageDescription</key>
<string>拍摄照片作为标注插图</string>

<!-- 开发期连本地 HTTP 后端时才需要，上架前必须去掉 -->
<key>NSAppTransportSecurity</key>
<dict><key>NSAllowsLocalNetworking</key><true/></dict>
```

Lapin 当前功能（阅读、标注、白板、讨论）**不需要任何敏感权限**，除非做图片上传或离线下载。权限越少审核越顺。

### 5.2 签名与证书

| 类型 | 用途 |
| --- | --- |
| Development | 真机调试 |
| Ad Hoc | 内部分发给已登记 UDID 的设备 |
| App Store | 提交 TestFlight / App Store |

推荐用 **fastlane match** 把证书和 Provisioning Profile 加密存进一个私有 git 仓库，团队和 CI 共享同一套，避免「证书在谁电脑上」的经典问题：

```sh
fastlane match init
fastlane match appstore     # 拉取/创建 App Store 证书与 profile
```

Associated Domains（深链接）需要在 `Runner.entitlements` 加 `applinks:lapin.example.com`，并在服务端提供 `/.well-known/apple-app-site-association`（无扩展名，`Content-Type: application/json`）。

### 5.3 账号与提交流程

1. Apple Developer Program，**每年 99 美元**（个人或公司；公司需要 D-U-N-S 号，申请可能耗时数周——要提前办）
2. App Store Connect 创建 App 记录，占用 Bundle ID
3. `flutter build ipa --release --export-options-plist=ios/ExportOptions.plist`
4. 上传：`xcrun altool --upload-app`（旧）或 `xcrun notarytool` / Transporter / fastlane `pilot`
5. TestFlight：内部测试（最多 100 人，无需审核，几分钟可用）；外部测试（最多 10000 人，首次需要轻量审核）
6. App Store 审核：通常 24–48 小时，被拒时在 Resolution Center 申诉

**给 Lapin 的具体审核风险**：如果 app 只是把网页内容包起来，会撞 App Store Review Guideline 4.2「最低功能要求」。必须有原生价值——白板手写、离线阅读、系统分享，这些正好是选 Flutter 的理由。

---

## 6. 与现有后端对接

以下全部基于当前 `main` 分支源码核实。

### 6.1 认证：必须用 Cookie + CSRF，不能用 Bearer（已核实）

这是全文最关键的一条。`internal/httpapi/server.go` 的路由表显示：

- `/api/v1/*` 全部挂 `RequireSession`，**写操作额外挂 `RequireCSRF()`**
- Bearer token（`RequireAccessToken()`）**只挂在 `/openapi/v1/*`**，而那组只有导入类接口（`subjects/import`、`subject-imports/*`、`assets`），**没有任何读取章节 / 标注 / 白板的接口**

所以 app 拿 `lpn_` token 是读不到业务数据的。具体机制（`internal/httpapi/handler/middleware.go`、`auth.go`）：

| 事实 | 值 |
| --- | --- |
| 会话 cookie | `lapin_session`，HttpOnly，SameSite=Lax，**7 天** |
| CSRF cookie | `lapin_csrf`，非 HttpOnly，同样 7 天 |
| CSRF 校验 | 双提交：`X-CSRF-Token` 头 == `lapin_csrf` cookie，且哈希匹配服务端会话记录 |
| Origin 校验 | 仅登录/注册接口检查；**Origin 为空时直接放行** |
| 访问令牌有效期 | 90 天（`tokens.go`，`createdAt.Add(90*24*time.Hour)`） |
| 响应封套 | 成功 `{"data": ...}`；失败 `{"error":{"code","message"}}` |

**原生 app 不发 Origin 头，所以登录能通过** —— 这点是好消息，不需要为移动端放宽校验。

落地方式：dio + 持久化 cookie jar，再加一个拦截器从 jar 里取 `lapin_csrf` 塞进头：

```dart
final jar = PersistCookieJar(storage: FileStorage('${dir.path}/.cookies'));
dio.interceptors.add(CookieManager(jar));
dio.interceptors.add(InterceptorsWrapper(
  onRequest: (options, handler) async {
    if (options.method != 'GET') {
      final cookies = await jar.loadForRequest(Uri.parse(options.baseUrl));
      final csrf = cookies.where((c) => c.name == 'lapin_csrf').firstOrNull;
      if (csrf != null) options.headers['X-CSRF-Token'] = csrf.value;
    }
    handler.next(options);
  },
  onError: (e, handler) {
    // 401 → 会话过期，跳登录页；403 + code=invalid_csrf 同理
    handler.next(e);
  },
));
```

**已知短板**：会话只有 7 天且没有刷新机制，用户每周要重新登录一次。两条改进路径（都需要改服务端，本方案不含实现）：

1. **让 `/api/v1` 也接受 Bearer**（推荐）：在 `RequireSession` 之外允许 `RequireAccessToken` 作为备选身份来源，app 登录后申请一个长期 token 存进 `flutter_secure_storage`。改动集中在中间件，且能顺带去掉移动端的 CSRF 负担（token 认证天然免疫 CSRF）。
2. 加一个刷新令牌端点，延长会话。

### 6.2 接口清单（app 需要的部分）

全部只有 GET 和 POST——这是仓库的硬约定（`CLAUDE.md`：不得新增 PUT/PATCH/DELETE）。

| 用途 | 方法与路径 |
| --- | --- |
| 注册 / 登录 / 登出 | `POST /api/v1/auth/{register,login,logout}` |
| 当前用户 | `GET /api/v1/me` |
| 科目列表 / 详情（含章节树） | `GET /api/v1/subjects`、`GET /api/v1/subjects/:id` |
| 标注读写 | `GET|POST /api/v1/chapters/:id/annotations` |
| 白板读写 | `GET /api/v1/chapters/:id/whiteboards`、`POST /api/v1/chapters/:id/whiteboard` |
| 讨论读写 | `GET|POST /api/v1/chapters/:id/comments` |
| 图片资源 | `GET /api/v1/assets/:id/content` |

所有 id 都是 HashID 字符串，客户端当不透明字符串处理即可，不要尝试解析成整数。

### 6.3 数据模型注意点

- 章节正文是 **Markdown 字符串**，服务端原样存取，渲染是客户端的事
- 标注：`note` 非空且 ≤2000 字符，`quote` ≤1000 字符，颜色只能是 `yellow|green|blue|pink`
- 章节树通过 `parent_id` 自引用，客户端要自己建树（Web 端的 `buildChapterTree` 可作参考）

### 6.4 标注锚定：不要照搬 Web 的字符偏移（已实测）

本仓库 2026-08-19 做过一次实测 spike，结论记录在 commit `777f1f7` 的提交说明里：同一章 Markdown，Web 端（Tiptap/marked）扁平化后 27,586 字符，Flutter 端（flutter_markdown_plus / dart markdown）27,612 字符，**26 处分歧**（粗体消解 11 处、`$` 6 处、有序列表序号 9 处）。前 8,494 字符（31%）逐字相同，所以只在章节开头抽查会得出「一致」的错误结论。

两边**都没有能对齐的配置开关**：Web 的 `markedOptions.gfm` 开关无效；Dart 的四套 `ExtensionSet`（none/commonMark/gitHubWeb/gitHubFlavored）输出完全相同。

好消息是 Flutter 的选区 API 完全够用：`SelectionListener` + `SelectionListenerNotifier` 给出的 `SelectedContentRange.startOffset/endOffset` 是相对整个子树、WidgetSpan 展平计入的偏移，语义与服务端一致（实测 select-all 报 `0..27612`，精确索引扁平化字符串）。

**因此 Flutter 端必须与 Web 端采用同一套锚定策略**：以 `quote` 文本定位，存储偏移仅用于在重复出现时消歧。Web 端已于 commit `777f1f7` 改成这样，移动端照做即可，服务端无需改动（它本来就存并校验 `quote`）。

### 6.5 白板：Flutter 上更简单，但格式被锁死

Web 端为了绕开浏览器 65535 像素的画布上限，做了一套「跟随阅读位置的窗口化画布」。**这些问题在 Flutter 上都不存在**：`CustomPaint` 只绘制视口内容，没有画布尺寸上限，也没有需要手动 refresh 的缓存偏移。手写笔迹用 `Listener` 收点 + `Path`，或 `perfect_freehand` 的 Dart 版。

但服务端 `validWhiteboardData` 的校验很严，Flutter 端必须产出**兼容 Excalidraw 的文档格式**：

| 约束 | 值 |
| --- | --- |
| 载荷上限 | 900,000 字节 |
| `space.width` / `space.height` | 100–10,000 / 100–200,000 |
| 元素坐标 | X 限 ±100,000，Y 与高度限 ±200,000 |
| 元素数量 | ≤5,000，每个元素序列化后 ≤100,000 字节 |
| 允许的元素类型 | `rectangle, diamond, ellipse, arrow, line, freedraw, text` |
| `appState` | 必须恰好一个键 `viewBackgroundColor: "transparent"` |
| `files` | 必须为空（不支持图片） |

现实选择只有一个：**复用 Excalidraw 文档格式，Flutter 端只渲染上述子集**。另起格式会被服务端拒绝。

### 6.6 推送与实时

现有后端**没有 WebSocket，也没有推送通道**（路由表里只有 REST）。如果要做「讨论有新回复」这类通知：

- 轻量做法：app 前台轮询 `GET /chapters/:id/comments`，代价是流量和延迟
- 正规做法：接 FCM（Android）+ APNs（iOS），服务端新增一个设备令牌注册端点和推送发送逻辑。iOS 推送需要 Apple Developer 账号下的 APNs 密钥，且 `Runner.entitlements` 要开 Push Notifications 能力

两者都需要服务端改动，不在本方案范围内。

---

## 7. CI：GitHub Actions 双平台构建

```yaml
name: mobile
on:
  push:
    tags: ['v*']
  pull_request:

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: subosito/flutter-action@v2
        with: { flutter-version: '3.44.9', channel: stable, cache: true }
      - run: flutter pub get
      - run: flutter analyze
      - run: flutter test

  android:
    needs: test
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-java@v4
        with: { distribution: temurin, java-version: '17' }
      - uses: subosito/flutter-action@v2
        with: { flutter-version: '3.44.9', channel: stable, cache: true }
      - name: 还原 keystore
        run: |
          echo "${{ secrets.ANDROID_KEYSTORE_BASE64 }}" | base64 -d > android/upload.jks
          cat > android/key.properties <<EOF
          storePassword=${{ secrets.ANDROID_STORE_PASSWORD }}
          keyPassword=${{ secrets.ANDROID_KEY_PASSWORD }}
          keyAlias=upload
          storeFile=${{ github.workspace }}/android/upload.jks
          EOF
      - run: flutter build appbundle --release --dart-define=API_BASE_URL=${{ vars.API_BASE_URL }}
      - uses: actions/upload-artifact@v4
        with: { name: aab, path: build/app/outputs/bundle/release/*.aab }

  ios:
    needs: test
    runs-on: macos-latest          # 私有仓库按 10 倍分钟数计费
    steps:
      - uses: actions/checkout@v4
      - uses: subosito/flutter-action@v2
        with: { flutter-version: '3.44.9', channel: stable, cache: true }
      - name: 导入签名（fastlane match）
        env:
          MATCH_PASSWORD: ${{ secrets.MATCH_PASSWORD }}
        run: |
          gem install fastlane
          fastlane match appstore --readonly
      - run: flutter build ipa --release --export-options-plist=ios/ExportOptions.plist
      - uses: actions/upload-artifact@v4
        with: { name: ipa, path: build/ios/ipa/*.ipa }
```

要点：keystore 与证书口令一律走 Secrets，绝不入库；`API_BASE_URL` 用 Repository Variables 区分环境；Flutter 版本号在三个 job 里必须一致，最好从 `.fvmrc` 读取避免漂移。

---

## 8. 发布与迭代

### 8.1 发版节奏

两端版本号保持一致（同一个 `pubspec.yaml` 的 `version`），但**发布时间必然错开**：Android 内部测试轨道分钟级可用，iOS 审核 1–2 天。实践做法是同一 tag 同时提交两端，Android 先在内部轨道放着，等 iOS 审核通过后一起放量，避免两端功能不一致导致用户困惑。

### 8.2 崩溃监控

| 方案 | 适用 |
| --- | --- |
| **Sentry**（`sentry_flutter`） | 自托管友好、Dart 异常与原生崩溃都抓、可与后端日志用同一套 |
| Firebase Crashlytics | 免费额度大、Android 生态整合好，但要引入整套 Firebase |

考虑到 Lapin 是自托管平台，**推荐 Sentry**（可自建 self-hosted 实例，数据不出内网）。务必上传混淆符号，否则栈全是乱码：

```sh
flutter build appbundle --release --obfuscate --split-debug-info=build/symbols
```

### 8.3 OTA 更新

Flutter 的 Dart 代码**不能**像 React Native 那样热更新——Apple 明确禁止下发可执行代码，Play 也不鼓励。可行的是：

- **内容级更新**：章节、标注这些本来就是从 API 拉的，改后端即可，无需发版。Lapin 的绝大多数迭代属于这类
- **强制升级提示**：加一个 `GET /api/v1/app-version` 之类的端点返回最低支持版本，app 启动时比对并提示去商店更新（需要服务端加端点）
- Shorebird 之类的第三方代码推送方案存在，但有合规风险，不建议用于上架应用

---

## 9. 落地顺序建议

| 阶段 | 内容 | 前置条件 |
| --- | --- | --- |
| 0 | 确认 iOS 构建资源（Mac 或 CI 预算）、注册两个开发者账号 | Apple 公司账号 D-U-N-S 申请可能耗时数周，最先启动 |
| 1 | 起工程骨架 + dio/cookie 认证打通，能登录并列出科目 | 无 |
| 2 | 章节阅读 + Markdown 渲染 + 标注（按 §6.4 用 quote 锚定） | 阶段 1 |
| 3 | 白板（`CustomPaint` + Excalidraw 兼容格式） | 阶段 2 |
| 4 | 讨论、离线缓存 | 阶段 2 |
| 5 | CI 双平台产物、TestFlight / 内部测试轨道 | 阶段 0 |

**建议在阶段 1 之前先做两个小验证**：(a) 用 dio + cookie jar 跑通一次登录并成功发起一个带 CSRF 的 POST；(b) 确认服务端是否要按 §6.1 的建议支持 Bearer——这个决定会影响整个认证层的写法，越早定越省事。

---

## 附：本文中已核实的事实与来源

| 结论 | 来源 |
| --- | --- |
| Flutter 3.44.9 / Dart 3.12.2 已装 | `/root/workspace/flutter/bin/flutter --version` |
| `/api/v1` 只认会话、Bearer 仅限 `/openapi/v1` | `internal/httpapi/server.go:51-88` |
| CSRF 双提交机制、Origin 空值放行 | `internal/httpapi/handler/middleware.go:118-135`、`auth.go:245-251` |
| 会话 7 天、令牌 90 天 | `auth.go:179-180`、`tokens.go:65` |
| 响应封套 `{data}` / `{error:{code,message}}` | `internal/httpapi/handler/response.go:27-33` |
| 白板校验约束 | `internal/httpapi/handler/interactions.go`（`validWhiteboardData`） |
| 标注偏移两端不一致、Flutter 选区 API 可用 | 2026-08-19 实测 spike，见 §6.4 |
