import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/network/api_client.dart';
import '../../auth/application/session_controller.dart';
import '../domain/annotation.dart';

class AnnotationRepository {
  const AnnotationRepository(this._client);

  final ApiClient _client;

  Future<List<Annotation>> list(String chapterId) async {
    final List<dynamic> data =
        await _client.get<List<dynamic>>('/api/v1/chapters/$chapterId/annotations');
    return <Annotation>[
      for (final dynamic item in data) Annotation.fromJson(item as Map<String, dynamic>),
    ];
  }

  /// Offsets are UTF-16 positions in the chapter's Markdown source, and [quote]
  /// must be the literal source substring they span — the server checks all
  /// three against each other and rejects any mismatch.
  Future<Annotation> create({
    required String chapterId,
    required int startOffset,
    required int endOffset,
    required String quote,
    required String note,
    required String color,
  }) async {
    final Map<String, dynamic> data = await _client.post<Map<String, dynamic>>(
      '/api/v1/chapters/$chapterId/annotations',
      body: <String, dynamic>{
        'start_offset': startOffset,
        'end_offset': endOffset,
        'quote': quote,
        'note': note,
        'color': color,
      },
    );
    return Annotation.fromJson(data);
  }
}

final Provider<AnnotationRepository> annotationRepositoryProvider = Provider<AnnotationRepository>(
  (Ref ref) => AnnotationRepository(ref.watch(apiClientProvider)),
);

/// Watches the session for the same reason the subject providers do: go_router
/// builds the initial route before its redirect runs, so an unauthenticated 401
/// would otherwise stay cached after sign-in.
final FutureProviderFamily<List<Annotation>, String> chapterAnnotationsProvider =
    FutureProvider.family<List<Annotation>, String>((Ref ref, String chapterId) {
  ref.watch(sessionProvider);
  return ref.watch(annotationRepositoryProvider).list(chapterId);
});
