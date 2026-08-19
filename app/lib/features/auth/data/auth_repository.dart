import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/network/api_client.dart';
import '../domain/session.dart';

class AuthRepository {
  const AuthRepository(this._client);

  final ApiClient _client;

  /// The response also sets `lapin_session` and `lapin_csrf`; the cookie jar
  /// picks both up, and the CSRF interceptor reads the second one from there on
  /// every later write. Both cookies live 7 days, after which the reader has to
  /// sign in again — the API has no refresh endpoint today.
  Future<User> signIn({required String email, required String password}) async {
    final Map<String, dynamic> data = await _client.post<Map<String, dynamic>>(
      '/api/v1/auth/login',
      body: <String, String>{'email': email, 'password': password},
    );
    return User.fromJson(data['user'] as Map<String, dynamic>);
  }

  /// Restores a session left in the cookie jar from a previous launch.
  Future<User?> currentUser() async {
    try {
      return User.fromJson(await _client.get<Map<String, dynamic>>('/api/v1/me'));
    } on Object {
      return null;
    }
  }

  Future<void> signOut() async {
    try {
      await _client.post<dynamic>('/api/v1/auth/logout');
    } finally {
      // Even if the call fails the local session is gone as far as the app is
      // concerned; leaving stale cookies behind would strand the reader.
      await _client.clearSession();
    }
  }
}

final Provider<AuthRepository> authRepositoryProvider = Provider<AuthRepository>(
  (Ref ref) => AuthRepository(ref.watch(apiClientProvider)),
);
