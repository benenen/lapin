import 'package:flutter/material.dart';
import 'package:flutter_markdown_plus/flutter_markdown_plus.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:markdown/markdown.dart' as md;

import '../../../core/config/env.dart';
import '../../../core/network/api_exception.dart';
import '../../annotations/data/annotation_repository.dart';
import '../../annotations/domain/annotation.dart';
import '../../annotations/domain/quote_anchor.dart';
import '../../annotations/presentation/annotation_sheets.dart';
import '../../annotations/presentation/annotation_spans.dart';
import '../../subjects/data/subject_repository.dart';
import '../../subjects/domain/subject.dart';

/// Reads one subject: a drawer with the chapter tree, and the selected
/// chapter's Markdown in the body, with annotation highlights on top.
///
/// TODO(whiteboard): the transparent drawing layer over the chapter. Must emit
/// the Excalidraw document shape the server validates. See
/// docs/flutter-app-plan.md §6.5.
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

class _ChapterScaffold extends ConsumerStatefulWidget {
  const _ChapterScaffold({
    required this.subject,
    required this.selectedChapterId,
    required this.onSelect,
  });

  final Subject subject;
  final String? selectedChapterId;
  final ValueChanged<String> onSelect;

  @override
  ConsumerState<_ChapterScaffold> createState() => _ChapterScaffoldState();
}

class _ChapterScaffoldState extends ConsumerState<_ChapterScaffold> {
  /// The reader's current selection, as rendered text plus the block it came
  /// from. Both are needed to map it back onto the Markdown source.
  String? _selectedText;
  String? _selectedBlock;

  late final AnnotationHighlightBuilder _highlights =
      AnnotationHighlightBuilder(onTap: _openAnnotationById);

  @override
  void dispose() {
    _highlights.dispose();
    super.dispose();
  }

  void _openAnnotationById(String id) {
    final Annotation? tapped = ref
        .read(chapterAnnotationsProvider(_currentChapterId))
        .valueOrNull
        ?.where((Annotation item) => item.id == id)
        .firstOrNull;
    if (tapped != null) {
      _showAnnotationDetail(tapped);
    }
  }

  String _currentChapterId = '';

  @override
  Widget build(BuildContext context) {
    final List<FlatChapter> chapters =
        flattenChapters(buildChapterTree(widget.subject.chapters));
    if (chapters.isEmpty) {
      return Scaffold(
        appBar: AppBar(title: Text(widget.subject.title), leading: const _BackToLibrary()),
        body: const Center(child: Text('这个课程还没有章节')),
      );
    }

    // A section heading carries no body of its own, so opening the subject on
    // one would show a blank page; start at the first chapter that has text.
    final FlatChapter current = chapters.firstWhere(
      (FlatChapter item) => item.chapter.id == widget.selectedChapterId,
      orElse: () => chapters.firstWhere(
        (FlatChapter item) => item.chapter.content.trim().isNotEmpty,
        orElse: () => chapters.first,
      ),
    );

    // A failed annotation load must not stop the reading: fall back to no
    // highlights rather than an error page.
    _currentChapterId = current.chapter.id;
    final List<Annotation> annotations =
        ref.watch(chapterAnnotationsProvider(current.chapter.id)).valueOrNull ?? <Annotation>[];
    final AnnotatedMarkdown prepared = annotateMarkdown(current.chapter.content, annotations);

    return Scaffold(
      appBar: AppBar(
        title: Text(current.chapter.title, overflow: TextOverflow.ellipsis),
        leading: const _BackToLibrary(),
        actions: <Widget>[
          IconButton(
            icon: const Icon(Icons.comment_outlined),
            tooltip: '本章标注',
            onPressed: () => _showAnnotationList(annotations),
          ),
        ],
      ),
      drawer: _ChapterDrawer(
        subject: widget.subject,
        chapters: chapters,
        currentId: current.chapter.id,
        onSelect: widget.onSelect,
      ),
      floatingActionButton: _selectedText == null
          ? null
          : FloatingActionButton.extended(
              icon: const Icon(Icons.edit_note),
              label: const Text('标注'),
              onPressed: () => _createAnnotation(current.chapter),
            ),
      body: current.chapter.content.trim().isEmpty
          ? const Center(child: Text('本章暂无正文'))
          : Markdown(
              data: prepared.markdown,
              selectable: true,
              padding: const EdgeInsets.fromLTRB(20, 16, 20, 48),
              styleSheet: MarkdownStyleSheet.fromTheme(Theme.of(context)).copyWith(
                p: Theme.of(context).textTheme.bodyLarge?.copyWith(height: 1.8),
              ),
              inlineSyntaxes: <md.InlineSyntax>[AnnotationSyntax(prepared.anchored)],
              builders: <String, MarkdownElementBuilder>{annotationTag: _highlights},
              onSelectionChanged: (String? text, TextSelection selection, SelectionChangedCause? cause) {
                final String? picked = (text != null && selection.isValid && !selection.isCollapsed)
                    ? selection.textInside(text)
                    : null;
                if (picked?.trim() == _selectedText) {
                  return;
                }
                setState(() {
                  _selectedText = picked?.trim().isEmpty ?? true ? null : picked!.trim();
                  _selectedBlock = _selectedText == null ? null : text;
                });
              },
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

  void _showAnnotationDetail(Annotation annotation) {
    showModalBottomSheet<void>(
      context: context,
      showDragHandle: true,
      builder: (BuildContext context) => AnnotationDetailSheet(annotation: annotation),
    );
  }

  void _showAnnotationList(List<Annotation> annotations) {
    showModalBottomSheet<void>(
      context: context,
      showDragHandle: true,
      builder: (BuildContext sheetContext) => AnnotationListSheet(
        annotations: annotations,
        onOpen: (Annotation annotation) {
          Navigator.of(sheetContext).pop();
          _showAnnotationDetail(annotation);
        },
      ),
    );
  }

  Future<void> _createAnnotation(Chapter chapter) async {
    final String? selected = _selectedText;
    if (selected == null) {
      return;
    }

    // The server stores offsets against the raw Markdown and checks that the
    // quote is a literal substring of it, so the rendered selection has to be
    // mapped back onto the source before anything is sent.
    final QuoteMatch? match =
        matchSelectionInSource(chapter.content, selected, blockText: _selectedBlock);
    if (match == null) {
      _tell('这段选中的文字在正文里定位不到，换一段再试');
      return;
    }
    if (match.text.length > annotationQuoteMaxLength) {
      _tell('选中的文字太长了，最多 $annotationQuoteMaxLength 个字符');
      return;
    }

    final NewAnnotation? input = await showModalBottomSheet<NewAnnotation>(
      context: context,
      isScrollControlled: true,
      showDragHandle: true,
      builder: (BuildContext context) => CreateAnnotationSheet(quote: match.text),
    );
    if (input == null) {
      return;
    }

    try {
      await ref.read(annotationRepositoryProvider).create(
            chapterId: chapter.id,
            startOffset: match.start,
            endOffset: match.end,
            quote: match.text,
            note: input.note,
            color: input.color,
          );
      ref.invalidate(chapterAnnotationsProvider(chapter.id));
      if (mounted) {
        setState(() {
          _selectedText = null;
          _selectedBlock = null;
        });
      }
      _tell('标注已保存');
    } on Object catch (error) {
      _tell(ApiException.from(error).message);
    }
  }

  void _tell(String message) {
    if (!mounted) {
      return;
    }
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(message)));
  }
}

class _ChapterDrawer extends StatelessWidget {
  const _ChapterDrawer({
    required this.subject,
    required this.chapters,
    required this.currentId,
    required this.onSelect,
  });

  final Subject subject;
  final List<FlatChapter> chapters;
  final String currentId;
  final ValueChanged<String> onSelect;

  @override
  Widget build(BuildContext context) => Drawer(
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
                      selected: item.chapter.id == currentId,
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
      );
}
