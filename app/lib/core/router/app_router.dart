import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../features/auth/application/session_controller.dart';
import '../../features/auth/presentation/sign_in_page.dart';
import '../../features/chapter/presentation/chapter_page.dart';
import '../../features/subjects/presentation/subject_list_page.dart';

/// Bridges a Riverpod provider to go_router's `refreshListenable`, so the
/// redirect below re-runs the moment the session appears or disappears.
class _SessionRefresh extends ChangeNotifier {
  _SessionRefresh(this._ref) {
    _ref.listen(sessionProvider, (_, _) => notifyListeners());
  }

  final Ref _ref;
}

final Provider<GoRouter> routerProvider = Provider<GoRouter>((Ref ref) {
  final _SessionRefresh refresh = _SessionRefresh(ref);
  ref.onDispose(refresh.dispose);

  return GoRouter(
    initialLocation: '/subjects',
    refreshListenable: refresh,
    redirect: (BuildContext context, GoRouterState state) {
      final AsyncValue<Object?> session = ref.read(sessionProvider);
      // Hold the current route while the stored session is being checked,
      // otherwise a cold start flashes the sign-in page at a signed-in reader.
      if (session.isLoading) return null;
      final bool signedIn = session.valueOrNull != null;
      final bool atSignIn = state.matchedLocation == '/sign-in';
      if (!signedIn) return atSignIn ? null : '/sign-in';
      if (atSignIn) return '/subjects';
      return null;
    },
    routes: <RouteBase>[
      GoRoute(path: '/sign-in', builder: (_, _) => const SignInPage()),
      GoRoute(path: '/subjects', builder: (_, _) => const SubjectListPage()),
      GoRoute(
        path: '/subjects/:id',
        builder: (BuildContext context, GoRouterState state) =>
            ChapterPage(subjectId: state.pathParameters['id']!),
      ),
    ],
  );
});
