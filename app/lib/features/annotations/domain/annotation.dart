/// One reader's note anchored to a stretch of chapter text.
///
/// The chapter's annotations are shared, not private: the server's list
/// endpoint does not filter by user, and every row carries its author's name.
class Annotation {
  const Annotation({
    required this.id,
    required this.chapterId,
    required this.authorName,
    required this.startOffset,
    required this.endOffset,
    required this.quote,
    required this.note,
    required this.color,
    required this.createdAt,
  });

  final String id;
  final String chapterId;
  final String authorName;

  /// UTF-16 offsets. This client measures them against the Markdown source,
  /// which is what the server validates. They only disambiguate a quote that
  /// occurs more than once — see quote_anchor.dart.
  final int startOffset;
  final int endOffset;

  final String quote;
  final String note;
  final String color;
  final DateTime createdAt;

  factory Annotation.fromJson(Map<String, dynamic> json) => Annotation(
        id: json['id'] as String,
        chapterId: json['chapter_id'] as String? ?? '',
        authorName: json['author_name'] as String? ?? '',
        startOffset: (json['start_offset'] as num?)?.toInt() ?? 0,
        endOffset: (json['end_offset'] as num?)?.toInt() ?? 0,
        quote: json['quote'] as String? ?? '',
        note: json['note'] as String? ?? '',
        color: json['color'] as String? ?? 'yellow',
        createdAt: DateTime.tryParse(json['created_at'] as String? ?? '')?.toLocal() ?? DateTime(1970),
      );
}

/// The colors the server accepts. Sending anything else is a 400.
const List<String> annotationColors = <String>['yellow', 'green', 'blue', 'pink'];

/// Server-side limits, mirrored so the form can stop a doomed request early.
const int annotationNoteMaxLength = 2000;
const int annotationQuoteMaxLength = 1000;
