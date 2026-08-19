import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/network/api_client.dart';
import '../domain/subject.dart';

class SubjectRepository {
  const SubjectRepository(this._client);

  final ApiClient _client;

  /// The list endpoint omits chapters; only the detail endpoint carries them.
  Future<List<Subject>> listSubjects() async {
    final List<dynamic> data = await _client.get<List<dynamic>>('/api/v1/subjects');
    return <Subject>[
      for (final dynamic subject in data) Subject.fromJson(subject as Map<String, dynamic>),
    ];
  }

  Future<Subject> getSubject(String id) async =>
      Subject.fromJson(await _client.get<Map<String, dynamic>>('/api/v1/subjects/$id'));
}

final Provider<SubjectRepository> subjectRepositoryProvider = Provider<SubjectRepository>(
  (Ref ref) => SubjectRepository(ref.watch(apiClientProvider)),
);

final FutureProviderFamily<Subject, String> subjectProvider =
    FutureProvider.family<Subject, String>(
  (Ref ref, String id) => ref.watch(subjectRepositoryProvider).getSubject(id),
);

final FutureProvider<List<Subject>> subjectsProvider = FutureProvider<List<Subject>>(
  (Ref ref) => ref.watch(subjectRepositoryProvider).listSubjects(),
);
