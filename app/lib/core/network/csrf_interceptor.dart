import 'package:cookie_jar/cookie_jar.dart';
import 'package:dio/dio.dart';

/// Every write under `/api/v1` is guarded by `RequireCSRF`, which checks that
/// the `X-CSRF-Token` header equals the `lapin_csrf` cookie *and* hashes to the
/// stored session. The web client reads that cookie from `document.cookie`;
/// here it comes out of the cookie jar.
///
/// Reads are left alone — the server only demands the header on writes.
class CsrfInterceptor extends Interceptor {
  CsrfInterceptor({required this.jar, required this.baseUrl});

  static const String cookieName = 'lapin_csrf';
  static const String headerName = 'X-CSRF-Token';

  final CookieJar jar;
  final String baseUrl;

  /// GET is the only method Lapin serves without a CSRF check; the API has no
  /// PUT/PATCH/DELETE at all, so everything else is a POST that needs one.
  static bool needsToken(String method) => method.toUpperCase() != 'GET';

  static String? tokenFrom(List<Cookie> cookies) {
    for (final Cookie cookie in cookies) {
      if (cookie.name == cookieName && cookie.value.isNotEmpty) return cookie.value;
    }
    return null;
  }

  @override
  Future<void> onRequest(RequestOptions options, RequestInterceptorHandler handler) async {
    if (needsToken(options.method)) {
      final String? token = tokenFrom(await jar.loadForRequest(Uri.parse(baseUrl)));
      if (token != null) options.headers[headerName] = token;
    }
    handler.next(options);
  }
}
