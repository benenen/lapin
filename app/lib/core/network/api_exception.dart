import 'package:dio/dio.dart';

/// Lapin answers with one of two envelopes:
///
/// * success — `{"data": ...}`
/// * failure — `{"error": {"code": "...", "message": "..."}}`
///
/// The message is already written for a reader, in Chinese, so it is shown as
/// it arrives instead of being re-worded here.
class ApiException implements Exception {
  const ApiException({required this.message, this.code, this.statusCode});

  final String message;
  final String? code;
  final int? statusCode;

  /// True when the session cookie is missing or has expired — the caller should
  /// send the reader back to the sign-in page rather than showing an error.
  bool get isUnauthenticated => statusCode == 401 || code == 'unauthenticated';

  /// The CSRF cookie and header stopped agreeing with the stored session. It is
  /// recoverable only by signing in again.
  bool get isInvalidCsrf => code == 'invalid_csrf' || statusCode == 403;

  factory ApiException.from(Object error) {
    if (error is ApiException) return error;
    if (error is! DioException) {
      return ApiException(message: '网络异常：$error');
    }
    final Response<dynamic>? response = error.response;
    final dynamic body = response?.data;
    if (body is Map && body['error'] is Map) {
      final Map<dynamic, dynamic> envelope = body['error'] as Map<dynamic, dynamic>;
      return ApiException(
        message: (envelope['message'] as String?) ?? '请求失败',
        code: envelope['code'] as String?,
        statusCode: response?.statusCode,
      );
    }
    return ApiException(
      message: switch (error.type) {
        DioExceptionType.connectionTimeout ||
        DioExceptionType.receiveTimeout ||
        DioExceptionType.sendTimeout =>
          '连接超时，请检查网络或服务地址',
        DioExceptionType.connectionError => '无法连接服务器，请确认后端已启动',
        _ => '请求失败（${response?.statusCode ?? '无响应'}）',
      },
      statusCode: response?.statusCode,
    );
  }

  @override
  String toString() => message;
}
