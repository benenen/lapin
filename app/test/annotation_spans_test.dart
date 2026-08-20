import 'package:flutter/material.dart';
import 'package:flutter_markdown_plus/flutter_markdown_plus.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:lapin_app/features/annotations/domain/annotation.dart';
import 'package:lapin_app/features/annotations/presentation/annotation_spans.dart';
import 'package:markdown/markdown.dart' as md;

Annotation makeAnnotation({
  String id = 'a1',
  String quote = '',
  int startOffset = 0,
  String note = '笔记',
  String color = 'yellow',
}) =>
    Annotation(
      id: id,
      chapterId: 'c1',
      authorName: '管理员',
      startOffset: startOffset,
      endOffset: startOffset + quote.length,
      quote: quote,
      note: note,
      color: color,
      createdAt: DateTime(2026, 8, 20),
    );

/// Runs the marked-up Markdown through the parser the widget uses.
List<md.Node> parse(AnnotatedMarkdown prepared) => md.Document(
      inlineSyntaxes: <md.InlineSyntax>[AnnotationSyntax(prepared.anchored)],
      extensionSet: md.ExtensionSet.gitHubFlavored,
    ).parse(prepared.markdown);

/// Depth-first search for the first element with [tag].
md.Element? findElement(List<md.Node> nodes, String tag) {
  for (final md.Node node in nodes) {
    if (node is md.Element) {
      if (node.tag == tag) {
        return node;
      }
      final md.Element? nested = findElement(node.children ?? <md.Node>[], tag);
      if (nested != null) {
        return nested;
      }
    }
  }
  return null;
}

String plainText(List<md.Node> nodes) => nodes.map((md.Node node) {
      if (node is md.Text) {
        return node.text;
      }
      if (node is md.Element) {
        return plainText(node.children ?? <md.Node>[]);
      }
      return '';
    }).join();

void main() {
  group('annotateMarkdown', () {
    test('把标记放在引用的那一处，正文其余部分原样保留', () {
      const String source = '这一段讲工具的用法。';
      final AnnotatedMarkdown prepared =
          annotateMarkdown(source, <Annotation>[makeAnnotation(quote: '工具', startOffset: source.indexOf('工具'))]);

      expect(prepared.anchored, hasLength(1));
      expect(prepared.markdown, isNot(source), reason: '应当插入了标记');
      expect(plainText(parse(prepared)), source, reason: '标记不能改变读者看到的文字');
    });

    test('引用出现多次时只标记 offset 指向的那一处', () {
      const String source = '第一段讲工具。第二段也讲工具。';
      final int second = source.lastIndexOf('工具');
      final AnnotatedMarkdown prepared =
          annotateMarkdown(source, <Annotation>[makeAnnotation(quote: '工具', startOffset: second)]);

      final List<md.Node> nodes = parse(prepared);
      final md.Element? link = findElement(nodes, 'a');
      expect(link, isNotNull);
      // 标记之前的文字里应当还留着第一处「工具」
      final int markerAt = prepared.markdown.indexOf('\u{E000}');
      expect(prepared.markdown.substring(0, markerAt), contains('工具'));
      expect(plainText(nodes), source);
    });

    test('生成 a > ann-<颜色> 结构，href 带得上标注 id', () {
      final AnnotatedMarkdown prepared = annotateMarkdown(
        '这一段讲工具的用法。',
        <Annotation>[makeAnnotation(id: 'xyz789', quote: '工具', startOffset: 4, color: 'green')],
      );
      final md.Element? link = findElement(parse(prepared), 'a');

      expect(link!.attributes['href'], 'lapin-annotation:xyz789');
      final md.Element inner = link.children!.single as md.Element;
      expect(inner.tag, 'ann-green');
      expect(plainText(<md.Node>[inner]), '工具');
    });

    test('引用已经不在正文里的标注被跳过，不影响其它标注', () {
      const String source = '正文只提到工具。';
      final AnnotatedMarkdown prepared = annotateMarkdown(source, <Annotation>[
        makeAnnotation(id: 'gone', quote: '早就删掉的句子'),
        makeAnnotation(id: 'kept', quote: '工具', startOffset: source.indexOf('工具')),
      ]);

      expect(prepared.anchored.map((Annotation a) => a.id), <String>['kept']);
      expect(plainText(parse(prepared)), source);
    });

    test('落在代码块里的引用被跳过，避免哨兵字符被当正文显示', () {
      const String source = '前言。\n\n```dart\nfinal tool = Tool();\n```\n\n后记。';
      final AnnotatedMarkdown prepared = annotateMarkdown(
        source,
        <Annotation>[makeAnnotation(quote: 'final tool', startOffset: source.indexOf('final tool'))],
      );

      expect(prepared.anchored, isEmpty);
      expect(prepared.markdown, source);
    });

    test('互相重叠的标注只标记先出现的那个', () {
      const String source = '讲工具的用法。';
      final AnnotatedMarkdown prepared = annotateMarkdown(source, <Annotation>[
        makeAnnotation(id: 'outer', quote: '工具的用法', startOffset: source.indexOf('工具')),
        makeAnnotation(id: 'inner', quote: '的用法', startOffset: source.indexOf('的用法')),
      ]);

      expect(prepared.anchored.map((Annotation a) => a.id), <String>['outer']);
      expect(plainText(parse(prepared)), source);
    });

    test('没有标注时原样返回', () {
      const String source = '# 标题\n\n正文。';
      expect(annotateMarkdown(source, <Annotation>[]).markdown, source);
    });

    test('引用里的 Markdown 语法不会破坏结构', () {
      const String source = '这里有 **加粗的工具** 一词。';
      final AnnotatedMarkdown prepared = annotateMarkdown(
        source,
        <Annotation>[makeAnnotation(quote: '**加粗的工具**', startOffset: source.indexOf('**'))],
      );
      final md.Element? link = findElement(parse(prepared), 'a');
      expect(link, isNotNull, reason: '仍应生成标注链接');
      expect(plainText(parse(prepared)), contains('加粗的工具'));
    });

    test('跨段落的标注不会把段落结构搅乱', () {
      const String source = '第一段。\n\n第二段。';
      final AnnotatedMarkdown prepared = annotateMarkdown(
        source,
        <Annotation>[makeAnnotation(quote: '第一段。\n\n第二段。')],
      );
      // 结构可以变（跨块的引用本来就没法用一个 span 表示），但文字不能丢。
      expect(plainText(parse(prepared)).replaceAll('\n', ''), contains('第一段'));
      expect(plainText(parse(prepared)).replaceAll('\n', ''), contains('第二段'));
    });
  });

  group('annotationIdFromHref', () {
    test('认得自家链接，放过普通链接', () {
      expect(annotationIdFromHref('lapin-annotation:abc'), 'abc');
      expect(annotationIdFromHref('https://example.com'), isNull);
      expect(annotationIdFromHref('lapin-annotation:'), isNull);
      expect(annotationIdFromHref(null), isNull);
    });
  });

  group('withAnnotationStyles', () {
    // Built through a real MaterialApp, the same way the chapter page does it.
    Future<Map<String, TextStyle?>> stylesFor(WidgetTester tester, Brightness brightness) async {
      late Map<String, TextStyle?> styles;
      await tester.pumpWidget(MaterialApp(
        theme: ThemeData(brightness: brightness),
        home: Builder(builder: (BuildContext context) {
          final ThemeData theme = Theme.of(context);
          styles = withAnnotationStyles(MarkdownStyleSheet.fromTheme(theme), brightness).styles;
          return const SizedBox.shrink();
        }),
      ));
      return styles;
    }

    testWidgets('给每种颜色都注册了样式，且覆盖链接色', (WidgetTester tester) async {
      final Map<String, TextStyle?> styles = await stylesFor(tester, Brightness.light);
      for (final String color in annotationColors) {
        final TextStyle? style = styles['ann-$color'];
        expect(style, isNotNull, reason: '$color 应当有高亮样式');
        expect(style!.backgroundColor, isNotNull);
        expect(style.decoration, TextDecoration.none, reason: '不能显示成下划线链接');
      }
    });

    testWidgets('深色模式下用半透明底色', (WidgetTester tester) async {
      final Color light = (await stylesFor(tester, Brightness.light))['ann-yellow']!.backgroundColor!;
      final Color dark = (await stylesFor(tester, Brightness.dark))['ann-yellow']!.backgroundColor!;
      expect(dark.a, lessThan(light.a));
    });
  });
}
