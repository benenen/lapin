import 'package:flutter/material.dart';
import 'package:flutter_markdown_plus/flutter_markdown_plus.dart';
import 'package:markdown/markdown.dart' as md;

import '../domain/annotation.dart';
import '../domain/quote_anchor.dart';

/// Renders annotation highlights as real [TextSpan]s inside the chapter text.
///
/// The quote is wrapped as `a > ann-<color>`: the `a` makes MarkdownBuilder
/// attach a tap recognizer, and the inner tag carries the highlight color from
/// the style sheet. Both stay text spans, so a highlight that runs past the end
/// of a line still wraps. A MarkdownElementBuilder cannot do this — it returns a
/// Widget, which becomes an atomic WidgetSpan that refuses to break.
///
/// Markers are inserted into the source rather than matching the quote text
/// directly, because inline syntaxes run per block: a match reports its offset
/// within the block, not within the chapter, so there is no way to compare it
/// against the stored offset from inside onMatch. Placing the marker at the one
/// offset we resolved up front makes disambiguation exact.
const String _markerOpen = '\u{E000}';
const String _markerMid = '\u{E001}';
const String _markerClose = '\u{E002}';

const String annotationLinkScheme = 'lapin-annotation';

/// Chapter Markdown with annotation markers inserted, plus the annotations that
/// actually got anchored — index-aligned with the markers.
class AnnotatedMarkdown {
  const AnnotatedMarkdown({required this.markdown, required this.anchored});

  const AnnotatedMarkdown.none(String source) : markdown = source, anchored = const <Annotation>[];

  final String markdown;
  final List<Annotation> anchored;
}

/// Places a marker around each annotation's quote in [source].
///
/// An annotation is skipped when its quote no longer appears (the chapter was
/// edited), when it lands inside a fenced code block (the marker would show up
/// as literal text), or when it overlaps one already placed.
AnnotatedMarkdown annotateMarkdown(String source, List<Annotation> annotations) {
  final List<({int start, int end, Annotation annotation})> spans =
      <({int start, int end, Annotation annotation})>[];
  final List<({int start, int end})> fences = _fencedCodeRanges(source);

  for (final Annotation annotation in annotations) {
    if (annotation.quote.isEmpty) {
      continue;
    }
    final int? start = locateQuote(source, annotation.quote, annotation.startOffset);
    if (start == null) {
      continue;
    }
    final int end = start + annotation.quote.length;
    if (fences.any((({int start, int end}) fence) => start < fence.end && fence.start < end)) {
      continue;
    }
    spans.add((start: start, end: end, annotation: annotation));
  }

  spans.sort((({int start, int end, Annotation annotation}) a,
          ({int start, int end, Annotation annotation}) b) =>
      a.start.compareTo(b.start));

  final StringBuffer buffer = StringBuffer();
  final List<Annotation> anchored = <Annotation>[];
  int cursor = 0;
  for (final ({int start, int end, Annotation annotation}) span in spans) {
    if (span.start < cursor) {
      continue; // Overlaps one already placed; nesting markers would corrupt both.
    }
    buffer
      ..write(source.substring(cursor, span.start))
      ..write(_markerOpen)
      ..write(anchored.length)
      ..write(_markerMid)
      ..write(source.substring(span.start, span.end))
      ..write(_markerClose);
    anchored.add(span.annotation);
    cursor = span.end;
  }
  buffer.write(source.substring(cursor));

  return AnnotatedMarkdown(markdown: buffer.toString(), anchored: anchored);
}

/// Start/end offsets of ``` fenced blocks, so markers stay out of them.
List<({int start, int end})> _fencedCodeRanges(String source) {
  final List<({int start, int end})> ranges = <({int start, int end})>[];
  int? openedAt;
  int offset = 0;
  for (final String line in source.split('\n')) {
    final bool isFence = line.trimLeft().startsWith('```');
    if (isFence) {
      if (openedAt == null) {
        openedAt = offset;
      } else {
        ranges.add((start: openedAt, end: offset + line.length));
        openedAt = null;
      }
    }
    offset += line.length + 1;
  }
  if (openedAt != null) {
    ranges.add((start: openedAt, end: source.length));
  }
  return ranges;
}

/// Turns the markers back into `a > ann-<color>` elements.
class AnnotationSyntax extends md.InlineSyntax {
  AnnotationSyntax(this.anchored)
      : super('$_markerOpen(\\d+)$_markerMid([\\s\\S]*?)$_markerClose',
            startCharacter: 0xE000);

  final List<Annotation> anchored;

  @override
  bool onMatch(md.InlineParser parser, Match match) {
    final int index = int.parse(match[1]!);
    final String quoted = match[2]!;
    if (index >= anchored.length) {
      parser.addNode(md.Text(quoted));
      return true;
    }
    final Annotation annotation = anchored[index];
    final md.Element link = md.Element('a', <md.Node>[
      md.Element.text('ann-${annotation.color}', quoted),
    ]);
    link.attributes['href'] = '$annotationLinkScheme:${annotation.id}';
    parser.addNode(link);
    return true;
  }
}

/// The annotation id in a tapped link, or null for an ordinary link.
String? annotationIdFromHref(String? href) {
  const String prefix = '$annotationLinkScheme:';
  if (href == null || !href.startsWith(prefix)) {
    return null;
  }
  final String id = href.substring(prefix.length);
  return id.isEmpty ? null : id;
}

const Map<String, Color> _highlightColors = <String, Color>{
  'yellow': Color(0xFFFFE082),
  'green': Color(0xFFA5D6A7),
  'blue': Color(0xFF90CAF9),
  'pink': Color(0xFFF48FB1),
};

/// Registers a style per annotation color.
///
/// MarkdownStyleSheet keeps its tag styles in a growable map and hands out that
/// same map, so custom tags can be added to it. Each style sets an explicit
/// foreground color to override the link color inherited from the wrapping `a`.
MarkdownStyleSheet withAnnotationStyles(MarkdownStyleSheet sheet, Brightness brightness) {
  final bool dark = brightness == Brightness.dark;
  for (final MapEntry<String, Color> entry in _highlightColors.entries) {
    sheet.styles['ann-${entry.key}'] = (sheet.p ?? const TextStyle()).copyWith(
      backgroundColor: dark ? entry.value.withValues(alpha: 0.32) : entry.value,
      color: dark ? Colors.white : Colors.black87,
      decoration: TextDecoration.none,
    );
  }
  return sheet;
}
