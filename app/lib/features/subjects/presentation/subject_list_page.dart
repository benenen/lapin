import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../auth/application/session_controller.dart';
import '../data/subject_repository.dart';
import '../domain/subject.dart';

class SubjectListPage extends ConsumerWidget {
  const SubjectListPage({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final AsyncValue<List<Subject>> subjects = ref.watch(subjectsProvider);

    return Scaffold(
      appBar: AppBar(
        title: const Text('我的课程'),
        actions: <Widget>[
          IconButton(
            tooltip: '退出登录',
            icon: const Icon(Icons.logout),
            onPressed: () => ref.read(sessionProvider.notifier).signOut(),
          ),
        ],
      ),
      body: RefreshIndicator(
        onRefresh: () async => ref.refresh(subjectsProvider.future),
        child: switch (subjects) {
          AsyncData<List<Subject>>(value: final List<Subject> items) when items.isEmpty =>
            const _Placeholder(icon: Icons.menu_book_outlined, message: '还没有课程'),
          AsyncData<List<Subject>>(value: final List<Subject> items) => ListView.separated(
              itemCount: items.length,
              separatorBuilder: (_, _) => const Divider(height: 1),
              itemBuilder: (BuildContext context, int index) {
                final Subject subject = items[index];
                return ListTile(
                  title: Text(subject.title),
                  subtitle: subject.description.isEmpty ? null : Text(
                    subject.description,
                    maxLines: 2,
                    overflow: TextOverflow.ellipsis,
                  ),
                  trailing: const Icon(Icons.chevron_right),
                  onTap: () => context.go('/subjects/${subject.id}'),
                );
              },
            ),
          AsyncError<List<Subject>>(error: final Object error) => _Placeholder(
              icon: Icons.error_outline,
              message: '$error',
              onRetry: () => ref.invalidate(subjectsProvider),
            ),
          _ => const Center(child: CircularProgressIndicator()),
        },
      ),
    );
  }
}

class _Placeholder extends StatelessWidget {
  const _Placeholder({required this.icon, required this.message, this.onRetry});

  final IconData icon;
  final String message;
  final VoidCallback? onRetry;

  @override
  Widget build(BuildContext context) => ListView(
        children: <Widget>[
          const SizedBox(height: 120),
          Icon(icon, size: 48, color: Theme.of(context).colorScheme.outline),
          const SizedBox(height: 16),
          Text(message, textAlign: TextAlign.center),
          if (onRetry != null) ...<Widget>[
            const SizedBox(height: 16),
            Center(child: OutlinedButton(onPressed: onRetry, child: const Text('重试'))),
          ],
        ],
      );
}
