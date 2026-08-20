import 'package:flutter/material.dart';

import '../domain/annotation.dart';

/// Shows one annotation: what was highlighted, what was written, and by whom.
///
/// Read-only on purpose — the server has no edit or delete route for
/// annotations, and HTTP routes here are limited to GET and POST.
class AnnotationDetailSheet extends StatelessWidget {
  const AnnotationDetailSheet({required this.annotation, super.key});

  final Annotation annotation;

  @override
  Widget build(BuildContext context) {
    final ThemeData theme = Theme.of(context);
    return SafeArea(
      child: Padding(
        padding: const EdgeInsets.fromLTRB(20, 16, 20, 24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: <Widget>[
            Container(
              padding: const EdgeInsets.all(12),
              decoration: BoxDecoration(
                color: theme.colorScheme.surfaceContainerHighest,
                borderRadius: BorderRadius.circular(8),
                border: Border(left: BorderSide(color: theme.colorScheme.primary, width: 3)),
              ),
              child: Text(annotation.quote, style: theme.textTheme.bodyMedium),
            ),
            const SizedBox(height: 16),
            Text(annotation.note, style: theme.textTheme.bodyLarge),
            const SizedBox(height: 16),
            Text(
              '${annotation.authorName} · ${_formatDate(annotation.createdAt)}',
              style: theme.textTheme.bodySmall?.copyWith(color: theme.colorScheme.outline),
            ),
          ],
        ),
      ),
    );
  }
}

/// Lists every annotation on the chapter. They are shared, not per-reader.
class AnnotationListSheet extends StatelessWidget {
  const AnnotationListSheet({required this.annotations, required this.onOpen, super.key});

  final List<Annotation> annotations;
  final ValueChanged<Annotation> onOpen;

  @override
  Widget build(BuildContext context) {
    if (annotations.isEmpty) {
      return const SafeArea(
        child: Padding(
          padding: EdgeInsets.all(32),
          child: Text('这一章还没有标注。选中正文里的文字就能写一条。', textAlign: TextAlign.center),
        ),
      );
    }
    return SafeArea(
      child: ListView.separated(
        shrinkWrap: true,
        padding: const EdgeInsets.symmetric(vertical: 8),
        itemCount: annotations.length,
        separatorBuilder: (BuildContext context, int index) => const Divider(height: 1),
        itemBuilder: (BuildContext context, int index) {
          final Annotation annotation = annotations[index];
          return ListTile(
            title: Text(annotation.note, maxLines: 2, overflow: TextOverflow.ellipsis),
            subtitle: Text(annotation.quote, maxLines: 1, overflow: TextOverflow.ellipsis),
            onTap: () => onOpen(annotation),
          );
        },
      ),
    );
  }
}

/// Collects the note and color for a new annotation.
///
/// Returns null when dismissed. The quote is already resolved to a literal
/// Markdown substring by the caller, so this form only gathers what the reader
/// still has to type.
class CreateAnnotationSheet extends StatefulWidget {
  const CreateAnnotationSheet({required this.quote, super.key});

  final String quote;

  @override
  State<CreateAnnotationSheet> createState() => _CreateAnnotationSheetState();
}

class NewAnnotation {
  const NewAnnotation({required this.note, required this.color});

  final String note;
  final String color;
}

class _CreateAnnotationSheetState extends State<CreateAnnotationSheet> {
  final TextEditingController _note = TextEditingController();
  String _color = annotationColors.first;

  @override
  void dispose() {
    _note.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final ThemeData theme = Theme.of(context);
    return Padding(
      // Keep the field above the keyboard.
      padding: EdgeInsets.only(bottom: MediaQuery.of(context).viewInsets.bottom),
      child: SafeArea(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(20, 16, 20, 20),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: <Widget>[
              Text('新建标注', style: theme.textTheme.titleMedium),
              const SizedBox(height: 12),
              Text(
                widget.quote,
                maxLines: 3,
                overflow: TextOverflow.ellipsis,
                style: theme.textTheme.bodySmall?.copyWith(color: theme.colorScheme.outline),
              ),
              const SizedBox(height: 16),
              TextField(
                controller: _note,
                autofocus: true,
                maxLines: 4,
                maxLength: annotationNoteMaxLength,
                decoration: const InputDecoration(
                  labelText: '笔记',
                  hintText: '写点什么',
                  border: OutlineInputBorder(),
                ),
                onChanged: (String _) => setState(() {}),
              ),
              Wrap(
                spacing: 8,
                children: <Widget>[
                  for (final String color in annotationColors)
                    ChoiceChip(
                      label: Text(_colorLabels[color] ?? color),
                      selected: _color == color,
                      onSelected: (bool _) => setState(() => _color = color),
                    ),
                ],
              ),
              const SizedBox(height: 16),
              Align(
                alignment: Alignment.centerRight,
                child: FilledButton(
                  // The server rejects an empty note, so do not offer to send one.
                  onPressed: _note.text.trim().isEmpty
                      ? null
                      : () => Navigator.of(context).pop(
                            NewAnnotation(note: _note.text.trim(), color: _color),
                          ),
                  child: const Text('保存'),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

const Map<String, String> _colorLabels = <String, String>{
  'yellow': '黄',
  'green': '绿',
  'blue': '蓝',
  'pink': '粉',
};

String _formatDate(DateTime time) =>
    '${time.year}-${_two(time.month)}-${_two(time.day)} ${_two(time.hour)}:${_two(time.minute)}';

String _two(int value) => value.toString().padLeft(2, '0');
