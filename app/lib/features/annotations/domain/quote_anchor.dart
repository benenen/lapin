/// Anchors annotations on their quoted text rather than on character offsets.
///
/// The offset-parity spike settled this: Flutter and the web client flatten the
/// same chapter Markdown to different strings (27,586 vs 27,612 characters on
/// 第 1 章, 26 divergences), and no parser setting closes the gap. The one thing
/// both clients hold verbatim is the Markdown source, so quotes are located in
/// the source and the stored offset serves only to disambiguate repeats.
///
/// Dart string indices are UTF-16 code units, which is exactly what the server
/// counts when it validates `end_offset - start_offset` against the quote.
library;

/// A stretch of the Markdown source that an annotation covers.
class QuoteMatch {
  const QuoteMatch({required this.start, required this.text});

  /// UTF-16 offset of the quote within the Markdown source.
  final int start;

  /// The literal source substring — never the rendered form, because the
  /// server checks `strings.Contains(chapter.Content, quote)`.
  final String text;

  int get end => start + text.length;

  @override
  String toString() => 'QuoteMatch($start, ${text.length} 字符)';
}

/// Finds [quote] in [source], preferring the occurrence nearest [hint].
///
/// Returns null when the quote is empty or absent — callers render the chapter
/// without that mark rather than guessing at a position.
int? locateQuote(String source, String quote, int hint) {
  if (quote.isEmpty) {
    return null;
  }
  if (hint >= 0 && hint <= source.length - quote.length && source.startsWith(quote, hint)) {
    return hint;
  }
  int? best;
  for (int index = source.indexOf(quote); index != -1; index = source.indexOf(quote, index + 1)) {
    if (best == null || (index - hint).abs() < (best - hint).abs()) {
      best = index;
    }
  }
  return best;
}

/// Turns a reader's selection — taken from *rendered* text — back into the
/// literal source substring the server will accept.
///
/// [blockText] is the rendered text of the block the selection came from. When
/// given, the search is narrowed to that block first, so selecting a word that
/// also appears earlier in the chapter still anchors where the reader selected.
QuoteMatch? matchSelectionInSource(String source, String selection, {String? blockText}) {
  final String wanted = selection.trim();
  if (wanted.isEmpty) {
    return null;
  }

  int windowStart = 0;
  String window = source;
  if (blockText != null && blockText.trim().isNotEmpty) {
    final QuoteMatch? block = _search(source, blockText.trim());
    if (block != null) {
      windowStart = block.start;
      window = block.text;
    }
  }

  final QuoteMatch? found = _search(window, wanted);
  if (found == null) {
    // The block narrowed us to the wrong place, or the block itself was not
    // found; fall back to the whole chapter rather than dropping the selection.
    return windowStart == 0 ? null : _search(source, wanted);
  }
  return QuoteMatch(start: windowStart + found.start, text: found.text);
}

/// Exact match first; then a whitespace-tolerant one.
///
/// A hard-wrapped paragraph renders with its newlines collapsed to spaces
/// (flutter_markdown_plus does this in `trimText` unless softLineBreak is on),
/// so a selection spanning a wrap never matches the source exactly. Matching
/// each whitespace run against `\s+` recovers it, and the returned text is
/// taken from the source so it stays a literal substring.
QuoteMatch? _search(String source, String wanted) {
  final int exact = source.indexOf(wanted);
  if (exact != -1) {
    return QuoteMatch(start: exact, text: wanted);
  }
  final List<String> parts = wanted.split(RegExp(r'\s+')).where((String part) => part.isNotEmpty).toList();
  if (parts.length < 2) {
    return null;
  }
  final RegExp relaxed = RegExp(parts.map(RegExp.escape).join(r'\s+'));
  final RegExpMatch? match = relaxed.firstMatch(source);
  return match == null ? null : QuoteMatch(start: match.start, text: match.group(0)!);
}
