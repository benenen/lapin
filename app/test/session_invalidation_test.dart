import 'package:cookie_jar/cookie_jar.dart';
import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:lapin_app/core/network/api_client.dart';
import 'package:lapin_app/features/auth/application/session_controller.dart';
import 'package:lapin_app/features/subjects/data/subject_repository.dart';
import 'package:lapin_app/features/subjects/domain/subject.dart';

/// Serves 401 until `signedIn` flips, mimicking the real sequence: the library
/// route is built before go_router's redirect runs, so the first request goes
/// out unauthenticated.
class _Backend implements HttpClientAdapter {
  bool signedIn = false;
  int subjectCalls = 0;

  @override
  Future<ResponseBody> fetch(RequestOptions options, Stream<dynamic>? stream, Future<void>? cancelFuture) async {
    final Map<String, List<String>> headers = <String, List<String>>{
      Headers.contentTypeHeader: <String>[Headers.jsonContentType],
    };
    if (options.path.endsWith('/auth/login')) {
      signedIn = true;
      return ResponseBody.fromString('{"data":{"user":{"id":"u1","email":"a@b.c","name":"A","roles":[]}}}', 200, headers: headers);
    }
    if (options.path.endsWith('/subjects')) {
      subjectCalls++;
      if (!signedIn) {
        return ResponseBody.fromString('{"error":{"code":"unauthenticated","message":"请先登录"}}', 401, headers: headers);
      }
      return ResponseBody.fromString('{"data":[{"id":"s1","title":"课程","description":"","tags":[],"chapters":[]}]}', 200, headers: headers);
    }
    // /me — nobody is signed in at boot.
    return ResponseBody.fromString('{"error":{"code":"unauthenticated","message":"请先登录"}}', 401, headers: headers);
  }

  @override
  void close({bool force = false}) {}
}

void main() {
  test('登录后课程列表会重新拉取，而不是留在登录前那次 401 上', () async {
    final _Backend backend = _Backend();
    final CookieJar jar = CookieJar();
    final ApiClient client = ApiClient(
      dio: ApiClient.buildDio(baseUrl: 'http://example.test', jar: jar)..httpClientAdapter = backend,
      jar: jar,
    );
    final ProviderContainer container = ProviderContainer(
      overrides: <Override>[apiClientProvider.overrideWithValue(client)],
    );
    addTearDown(container.dispose);

    // 会话恢复失败 —— 未登录状态。
    await container.read(sessionProvider.future);

    // 路由重定向生效前，课程页已经挂载过一次，这一次必然 401。
    await expectLater(container.read(subjectsProvider.future), throwsA(anything));
    expect(backend.subjectCalls, 1);

    await container.read(sessionProvider.notifier).signIn(email: 'a@b.c', password: 'x');

    // 关键：登录后必须重新发请求，而不是复用缓存里的错误。
    final List<Subject> subjects = await container.read(subjectsProvider.future);
    expect(subjects, hasLength(1));
    expect(backend.subjectCalls, 2, reason: '会话变化应让课程列表失效并重新拉取');
  });
}
