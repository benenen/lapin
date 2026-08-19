import 'dart:typed_data';

import 'package:cookie_jar/cookie_jar.dart';
import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:lapin_app/core/network/api_client.dart';
import 'package:lapin_app/core/network/api_exception.dart';
import 'package:lapin_app/core/network/csrf_interceptor.dart';

/// Answers requests from memory so the tests exercise the interceptor chain and
/// the envelope handling without a server.
class _StubAdapter implements HttpClientAdapter {
  _StubAdapter(this.respond);

  final ResponseBody Function(RequestOptions options) respond;
  final List<RequestOptions> seen = <RequestOptions>[];

  @override
  Future<ResponseBody> fetch(RequestOptions options, Stream<Uint8List>? stream, Future<void>? cancelFuture) async {
    seen.add(options);
    return respond(options);
  }

  @override
  void close({bool force = false}) {}
}

ResponseBody json(String body, {int status = 200, Map<String, List<String>>? headers}) =>
    ResponseBody.fromString(
      body,
      status,
      headers: <String, List<String>>{
        Headers.contentTypeHeader: <String>[Headers.jsonContentType],
        ...?headers,
      },
    );

void main() {
  group('CSRF 拦截器', () {
    test('写请求只在 GET 之外触发', () {
      expect(CsrfInterceptor.needsToken('GET'), isFalse);
      expect(CsrfInterceptor.needsToken('get'), isFalse);
      expect(CsrfInterceptor.needsToken('POST'), isTrue);
    });

    test('从 cookie 罐里取 lapin_csrf', () {
      final List<Cookie> cookies = <Cookie>[
        Cookie('lapin_session', 'session-value'),
        Cookie('lapin_csrf', 'csrf-value'),
      ];

      expect(CsrfInterceptor.tokenFrom(cookies), 'csrf-value');
      expect(CsrfInterceptor.tokenFrom(<Cookie>[Cookie('lapin_session', 'x')]), isNull);
    });

    test('POST 会带上 X-CSRF-Token，GET 不带', () async {
      final CookieJar jar = CookieJar();
      final Uri origin = Uri.parse('http://example.test');
      await jar.saveFromResponse(origin, <Cookie>[Cookie('lapin_csrf', 'csrf-value')]);

      final _StubAdapter adapter = _StubAdapter((_) => json('{"data":{}}'));
      final Dio dio = ApiClient.buildDio(baseUrl: origin.toString(), jar: jar)
        ..httpClientAdapter = adapter;
      final ApiClient client = ApiClient(dio: dio, jar: jar);

      await client.post<Map<String, dynamic>>('/api/v1/chapters/x/comments', body: <String, String>{'body': 'hi'});
      await client.get<Map<String, dynamic>>('/api/v1/me');

      expect(adapter.seen.first.headers[CsrfInterceptor.headerName], 'csrf-value');
      expect(adapter.seen.last.headers.containsKey(CsrfInterceptor.headerName), isFalse);
    });

    test('还没有 cookie 时不会伪造一个空令牌', () async {
      final CookieJar jar = CookieJar();
      final _StubAdapter adapter = _StubAdapter((_) => json('{"data":{}}'));
      final Dio dio = ApiClient.buildDio(baseUrl: 'http://example.test', jar: jar)
        ..httpClientAdapter = adapter;

      await ApiClient(dio: dio, jar: jar).post<Map<String, dynamic>>('/api/v1/auth/login');

      expect(adapter.seen.single.headers.containsKey(CsrfInterceptor.headerName), isFalse);
    });
  });

  group('响应封套', () {
    ApiClient clientFor(ResponseBody Function(RequestOptions) respond) {
      final CookieJar jar = CookieJar();
      final Dio dio = ApiClient.buildDio(baseUrl: 'http://example.test', jar: jar)
        ..httpClientAdapter = _StubAdapter(respond);
      return ApiClient(dio: dio, jar: jar);
    }

    test('成功时剥掉 data 外层', () async {
      final Map<String, dynamic> user = await clientFor((_) => json('{"data":{"id":"abc","email":"a@b.c"}}'))
          .get<Map<String, dynamic>>('/api/v1/me');

      expect(user['id'], 'abc');
    });

    test('失败时把 error 封套转成带原文案的异常', () async {
      final ApiClient client = clientFor(
        (_) => json('{"error":{"code":"invalid_credentials","message":"邮箱或密码不正确"}}', status: 401),
      );

      await expectLater(
        client.post<Map<String, dynamic>>('/api/v1/auth/login'),
        throwsA(
          isA<ApiException>()
              .having((ApiException e) => e.message, 'message', '邮箱或密码不正确')
              .having((ApiException e) => e.code, 'code', 'invalid_credentials')
              .having((ApiException e) => e.isUnauthenticated, 'isUnauthenticated', isTrue),
        ),
      );
    });

    test('CSRF 失效可以被单独识别出来，以便引导重新登录', () async {
      final ApiClient client = clientFor(
        (_) => json('{"error":{"code":"invalid_csrf","message":"请求校验失败"}}', status: 403),
      );

      await expectLater(
        client.post<Map<String, dynamic>>('/api/v1/chapters/x/comments'),
        throwsA(isA<ApiException>().having((ApiException e) => e.isInvalidCsrf, 'isInvalidCsrf', isTrue)),
      );
    });

    test('连不上服务器时给的是人话，不是 DioException', () async {
      final CookieJar jar = CookieJar();
      final Dio dio = ApiClient.buildDio(baseUrl: 'http://example.test', jar: jar)
        ..httpClientAdapter = _StubAdapter((RequestOptions options) => throw DioException(
              requestOptions: options,
              type: DioExceptionType.connectionError,
            ));

      await expectLater(
        ApiClient(dio: dio, jar: jar).get<Map<String, dynamic>>('/api/v1/me'),
        throwsA(isA<ApiException>().having((ApiException e) => e.message, 'message', contains('无法连接服务器'))),
      );
    });
  });
}
