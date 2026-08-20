@Tags(<String>['live'])
library;

import 'package:cookie_jar/cookie_jar.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:lapin_app/core/network/api_client.dart';

/// Exercises the real cookie + CSRF handshake against a running backend.
///
/// Skipped unless a server is reachable, so `flutter test` stays hermetic:
///
/// ```sh
/// flutter test test/live_session_test.dart --dart-define=LIVE_BASE_URL=http://127.0.0.1:8080
/// ```
const String baseUrl = String.fromEnvironment('LIVE_BASE_URL');

void main() {
  test('登录后拿到的 cookie 会被带到后续请求', () async {
    final CookieJar jar = CookieJar();
    final ApiClient client = ApiClient(
      dio: ApiClient.buildDio(baseUrl: baseUrl, jar: jar),
      jar: jar,
    );

    final Map<String, dynamic> login = await client.post<Map<String, dynamic>>(
      '/api/v1/auth/login',
      body: <String, String>{'email': 'admin@localhost', 'password': 'admin12345678'},
    );
    expect(login['user'], isNotNull, reason: '登录应返回用户');

    final List<Cookie> stored = await jar.loadForRequest(Uri.parse(baseUrl));
    expect(
      stored.map((Cookie cookie) => cookie.name),
      containsAll(<String>['lapin_session', 'lapin_csrf']),
      reason: 'cookie 罐应收下会话与 CSRF 两个 cookie',
    );

    // 关键一步：这个请求必须带着上一步的 cookie，否则服务端返回 401。
    final Map<String, dynamic> me = await client.get<Map<String, dynamic>>('/api/v1/me');
    expect(me['email'], 'admin@localhost');

    final List<dynamic> subjects = await client.get<List<dynamic>>('/api/v1/subjects');
    expect(subjects, isNotEmpty, reason: '管理员至少能看到一个课程');
  }, skip: baseUrl.isEmpty ? '未提供 LIVE_BASE_URL，跳过联机用例' : null);
}
