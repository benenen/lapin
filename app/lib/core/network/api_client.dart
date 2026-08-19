import 'package:cookie_jar/cookie_jar.dart';
import 'package:dio/dio.dart';
import 'package:dio_cookie_manager/dio_cookie_manager.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:path_provider/path_provider.dart';

import '../config/env.dart';
import 'api_exception.dart';
import 'csrf_interceptor.dart';

/// Talks to Lapin's `/api/v1`.
///
/// That surface is session-authenticated: sign-in sets `lapin_session` and
/// `lapin_csrf` cookies, and every write echoes the CSRF cookie back in a
/// header. Bearer access tokens exist but are only accepted on `/openapi/v1`,
/// which exposes no read endpoints — so a cookie jar is the only way in.
///
/// A native client sends no `Origin` header, and the server's origin check
/// passes when it is absent, so no server change is needed to sign in.
class ApiClient {
  ApiClient({required this.dio, required this.jar});

  final Dio dio;
  final CookieJar jar;

  static Future<ApiClient> create({String baseUrl = Env.apiBaseUrl}) async {
    final support = await getApplicationSupportDirectory();
    final PersistCookieJar jar = PersistCookieJar(
      ignoreExpires: false,
      storage: FileStorage('${support.path}/.cookies'),
    );
    return ApiClient(dio: buildDio(baseUrl: baseUrl, jar: jar), jar: jar);
  }

  /// Split out so tests can build a client over an in-memory jar.
  static Dio buildDio({required String baseUrl, required CookieJar jar}) {
    final Dio dio = Dio(
      BaseOptions(
        baseUrl: baseUrl,
        connectTimeout: const Duration(seconds: 10),
        receiveTimeout: const Duration(seconds: 20),
        contentType: Headers.jsonContentType,
        // Envelopes carry the failure detail, so let this layer read the body
        // instead of dio turning every 4xx into a bare exception.
        validateStatus: (int? status) => status != null && status < 500,
      ),
    );
    dio.interceptors.add(CookieManager(jar));
    dio.interceptors.add(CsrfInterceptor(jar: jar, baseUrl: baseUrl));
    return dio;
  }

  Future<T> get<T>(String path) => _unwrap<T>(dio.get<dynamic>(path));

  Future<T> post<T>(String path, {Object? body}) =>
      _unwrap<T>(dio.post<dynamic>(path, data: body ?? const <String, dynamic>{}));

  Future<T> _unwrap<T>(Future<Response<dynamic>> request) async {
    late final Response<dynamic> response;
    try {
      response = await request;
    } on DioException catch (error) {
      throw ApiException.from(error);
    }
    final dynamic body = response.data;
    if (body is Map && body['error'] is Map) {
      final Map<dynamic, dynamic> envelope = body['error'] as Map<dynamic, dynamic>;
      throw ApiException(
        message: (envelope['message'] as String?) ?? '请求失败',
        code: envelope['code'] as String?,
        statusCode: response.statusCode,
      );
    }
    if ((response.statusCode ?? 500) >= 400) {
      throw ApiException(message: '请求失败（${response.statusCode}）', statusCode: response.statusCode);
    }
    if (body is Map && body.containsKey('data')) return body['data'] as T;
    return body as T;
  }

  Future<void> clearSession() => jar.deleteAll();
}

final Provider<ApiClient> apiClientProvider = Provider<ApiClient>((Ref ref) {
  throw UnimplementedError('apiClientProvider must be overridden in main()');
});
