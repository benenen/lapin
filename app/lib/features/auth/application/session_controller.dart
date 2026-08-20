import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../data/auth_repository.dart';
import '../domain/session.dart';

/// Holds the signed-in user, or null when nobody is. The router watches this to
/// decide between the sign-in page and the library.
class SessionController extends AsyncNotifier<User?> {
  @override
  Future<User?> build() => ref.watch(authRepositoryProvider).currentUser();

  Future<void> signIn({required String email, required String password}) async {
    state = const AsyncValue<User?>.loading();
    state = await AsyncValue.guard<User?>(
      () => ref.read(authRepositoryProvider).signIn(email: email, password: password),
    );
  }

  Future<void> signOut() async {
    await ref.read(authRepositoryProvider).signOut();
    state = const AsyncValue<User?>.data(null);
  }
}

final AsyncNotifierProvider<SessionController, User?> sessionProvider =
    AsyncNotifierProvider<SessionController, User?>(SessionController.new);
