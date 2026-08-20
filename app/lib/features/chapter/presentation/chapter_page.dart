import 'package:flutter/material.dart';
import 'package:flutter_markdown_plus/flutter_markdown_plus.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../core/config/env.dart';
import '../../subjects/data/subject_repository.dart';
import '../../subjects/domain/subject.dart';

/// Reads one subject: a drawer with the chapter tree, and the selected
/// chapter's Markdown in the body.
///
/// TODO(annotations): render annotation marks and let the reader create one.
/// Anchor them on the stored `quote`, not on the character offsets — the web
/// client and this one flatten the same Markdown to different strings (27,586
/// vs 27,612 characters on 第 1 章), and no parser setting closes that gap. See
/// docs/flutter-app-plan.md §6.4.
/// TODO(whiteboard): the transparent drawing layer over the chapter. Must emit
/// the Excalidraw document shape the server validates. See §6.5.
class ChapterPage extends ConsumerStatefulWidget {
  const ChapterPage({required this.subjectId, super.key});

  final String subjectId;

  @override
  ConsumerState<ChapterPage> createState() => _ChapterPageState();
}

class _ChapterPageState extends ConsumerState<ChapterPage> {
  String? _selectedChapterId;

  @override
  Widget build(BuildContext context) {
    final AsyncValue<Subject> subject = ref.watch(subjectProvider(widget.subjectId));

    return switch (subject) {
      AsyncData<Subject>(value: final Subject data) => _ChapterScaffold(
          subject: data,
          selectedChapterId: _selectedChapterId,
          onSelect: (String id) => setState(() => _selectedChapterId = id),
        ),
      AsyncError<Subject>(error: final Object error) => Scaffold(
          appBar: AppBar(leading: const _BackToLibrary()),
          body: Center(
            child: Padding(padding: const EdgeInsets.all(24), child: Text('$error')),
          ),
        ),
      _ => Scaffold(
          appBar: AppBar(leading: const _BackToLibrary()),
          body: const Center(child: CircularProgressIndicator()),
        ),
    };
  }
}

class _BackToLibrary extends StatelessWidget {
  const _BackToLibrary();

  @override
  Widget build(BuildContext context) => IconButton(
        icon: const Icon(Icons.arrow_back),
        onPressed: () => context.go('/subjects'),
      );
}

class _ChapterScaffold extends StatelessWidget {
  const _ChapterScaffold({
    required this.subject,
    required this.selectedChapterId,
    required this.onSelect,
  });

  final Subject subject;
  final String? selectedChapterId;
  final ValueChanged<String> onSelect;

  @override
  Widget build(BuildContext context) {
    final List<FlatChapter> chapters = flattenChapters(buildChapterTree(subject.chapters));
    if (chapters.isEmpty) {
      return Scaffold(
        appBar: AppBar(title: Text(subject.title), leading: const _BackToLibrary()),
        body: const Center(child: Text('这个课程还没有章节')),
      );
    }

    // A section heading carries no body of its own, so opening the subject on
    // one would show a blank page; start at the first chapter that has text.
    final FlatChapter current = chapters.firstWhere(
      (FlatChapter item) => item.chapter.id == selectedChapterId,
      orElse: () => chapters.firstWhere(
        (FlatChapter item) => item.chapter.content.trim().isNotEmpty,
        orElse: () => chapters.first,
      ),
    );

    return Scaffold(
      appBar: AppBar(
        title: Text(current.chapter.title, overflow: TextOverflow.ellipsis),
        leading: const _BackToLibrary(),
      ),
      drawer: Drawer(
        child: SafeArea(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: <Widget>[
              Padding(
                padding: const EdgeInsets.all(16),
                child: Text(subject.title, style: Theme.of(context).textTheme.titleMedium),
              ),
              const Divider(height: 1),
              Expanded(
                child: ListView.builder(
                  itemCount: chapters.length,
                  itemBuilder: (BuildContext context, int index) {
                    final FlatChapter item = chapters[index];
                    return ListTile(
                      selected: item.chapter.id == current.chapter.id,
                      contentPadding: EdgeInsets.only(left: 16.0 + item.depth * 16, right: 16),
                      title: Text(item.chapter.title, style: Theme.of(context).textTheme.bodyMedium),
                      onTap: () {
                        onSelect(item.chapter.id);
                        Navigator.of(context).pop();
                      },
                    );
                  },
                ),
              ),
            ],
          ),
        ),
      ),
      body: current.chapter.content.trim().isEmpty
          ? const Center(child: Text('本章暂无正文'))
          : Markdown(
              data: current.chapter.content,
              selectable: true,
              padding: const EdgeInsets.fromLTRB(20, 16, 20, 48),
              styleSheet: MarkdownStyleSheet.fromTheme(Theme.of(context)).copyWith(
                p: Theme.of(context).textTheme.bodyLarge?.copyWith(height: 1.8),
              ),
              // Chapter images are served from the same API, and their src is a
              // root-relative path, so resolve it against the configured origin.
              imageBuilder: (Uri uri, String? title, String? alt) {
                final Uri resolved = uri.hasScheme ? uri : Uri.parse('${Env.apiBaseUrl}$uri');
                return Image.network(
                  resolved.toString(),
                  errorBuilder: (_, _, _) => const SizedBox.shrink(),
                );
              },
            ),
    );
  }
}
