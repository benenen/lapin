import 'package:cookie_jar/cookie_jar.dart';
import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:lapin_app/core/network/api_client.dart';
import 'package:flutter_markdown_plus/flutter_markdown_plus.dart';
import 'package:lapin_app/features/chapter/presentation/chapter_page.dart';

const String chapterBody = '这一段讲工具的用法，后面还会再提到。';

/// Serves just enough of the API for the chapter page to render.
class FakeBackend implements HttpClientAdapter {
  int annotationPosts = 0;
  Map<String, dynamic>? lastPostedBody;

  @override
  Future<ResponseBody> fetch(
      RequestOptions options, Stream<dynamic>? stream, Future<void>? cancelFuture) async {
    final Map<String, List<String>> headers = <String, List<String>>{
      Headers.contentTypeHeader: <String>[Headers.jsonContentType],
    };
    String json;
    int status = 200;
    if (options.path.endsWith('/subjects/s1')) {
      json = '{"data":{"id":"s1","title":"课程","description":"","tags":[],"chapters":['
          '{"id":"c1","subject_id":"s1","title":"引言","content":"$chapterBody","position":1}]}}';
    } else if (options.path.endsWith('/annotations') && options.method == 'POST') {
      annotationPosts++;
      lastPostedBody = Map<String, dynamic>.from(options.data as Map<dynamic, dynamic>);
      status = 201;
      json = '{"data":${_annotationJson('new', '后面还会再提到')}}';
    } else if (options.path.endsWith('/annotations')) {
      json = '{"data":[${_annotationJson('a1', '工具的用法')}]}';
    } else {
      json = '{"error":{"code":"unauthenticated","message":"请先登录"}}';
      status = 401;
    }
    return ResponseBody.fromString(json, status, headers: headers);
  }

  String _annotationJson(String id, String quote) =>
      '{"id":"$id","chapter_id":"c1","user_id":"u1","author_name":"管理员",'
      '"start_offset":${chapterBody.indexOf(quote)},"end_offset":${chapterBody.indexOf(quote) + quote.length},'
      '"quote":"$quote","note":"这里是笔记正文","color":"yellow","created_at":"2026-08-20T10:00:00Z"}';

  @override
  void close({bool force = false}) {}
}

Future<FakeBackend> pumpChapter(WidgetTester tester) async {
  final FakeBackend backend = FakeBackend();
  final CookieJar jar = CookieJar();
  final ApiClient client = ApiClient(
    dio: ApiClient.buildDio(baseUrl: 'http://example.test', jar: jar)..httpClientAdapter = backend,
    jar: jar,
  );

  await tester.pumpWidget(ProviderScope(
    overrides: <Override>[apiClientProvider.overrideWithValue(client)],
    child: const MaterialApp(home: ChapterPage(subjectId: 's1')),
  ));
  await tester.pumpAndSettle();
  return backend;
}

void main() {
  testWidgets('章节正文与标注一起加载出来', (WidgetTester tester) async {
    await pumpChapter(tester);

    expect(find.text('引言'), findsOneWidget, reason: 'AppBar 应显示章节名');
    expect(find.byIcon(Icons.comment_outlined), findsOneWidget, reason: '应有查看本章标注的入口');
    expect(find.byType(FloatingActionButton), findsNothing, reason: '没有选中文字时不该出现标注按钮');
  });

  testWidgets('标注列表能打开，点进去看到引用、笔记和作者', (WidgetTester tester) async {
    await pumpChapter(tester);

    await tester.tap(find.byIcon(Icons.comment_outlined));
    await tester.pumpAndSettle();
    expect(find.text('这里是笔记正文'), findsOneWidget);

    await tester.tap(find.text('这里是笔记正文'));
    await tester.pumpAndSettle();
    expect(find.text('工具的用法'), findsOneWidget, reason: '详情里应显示被标注的原文');
    expect(find.textContaining('管理员'), findsOneWidget, reason: '标注是共享的，要显示作者');
  });

  testWidgets('选中文字后能新建标注，请求体带的是 Markdown 源上的偏移', (WidgetTester tester) async {
    final FakeBackend backend = await pumpChapter(tester);

    // 真实的长按拖拽选区在测试里不可靠，直接触发 Markdown 的选区回调，
    // 验证从选区到请求体这条链路。
    const String picked = '后面还会再提到';
    final int at = chapterBody.indexOf(picked);
    tester.widget<Markdown>(find.byType(Markdown)).onSelectionChanged!(
          chapterBody,
          TextSelection(baseOffset: at, extentOffset: at + picked.length),
          SelectionChangedCause.longPress,
        );
    await tester.pumpAndSettle();

    expect(find.byType(FloatingActionButton), findsOneWidget, reason: '有选区时才出现标注按钮');
    await tester.tap(find.byType(FloatingActionButton));
    await tester.pumpAndSettle();

    expect(find.text('新建标注'), findsOneWidget);
    expect(find.text(picked), findsOneWidget, reason: '表单里应回显被选中的文字');

    await tester.enterText(find.byType(TextField), '手机上写的笔记');
    await tester.pumpAndSettle();
    await tester.tap(find.widgetWithText(FilledButton, '保存'));
    await tester.pumpAndSettle();

    expect(backend.annotationPosts, 1);
    expect(backend.lastPostedBody!['quote'], picked);
    expect(backend.lastPostedBody!['start_offset'], at, reason: '偏移基准是章节 Markdown 源');
    expect(backend.lastPostedBody!['end_offset'], at + picked.length);
    expect(backend.lastPostedBody!['note'], '手机上写的笔记');
    expect(backend.lastPostedBody!['color'], 'yellow');
  });

  testWidgets('选中的文字在正文里定位不到时不发请求', (WidgetTester tester) async {
    final FakeBackend backend = await pumpChapter(tester);

    tester.widget<Markdown>(find.byType(Markdown)).onSelectionChanged!(
          '这段块文本并不属于章节正文',
          const TextSelection(baseOffset: 0, extentOffset: 6),
          SelectionChangedCause.longPress,
        );
    await tester.pumpAndSettle();
    await tester.tap(find.byType(FloatingActionButton));
    await tester.pumpAndSettle();

    expect(find.text('新建标注'), findsNothing, reason: '定位不到就不该进到表单');
    expect(find.textContaining('定位不到'), findsOneWidget, reason: '要明确告诉用户重选');
    expect(backend.annotationPosts, 0);
  });

  testWidgets('标注加载失败时正文照常可读', (WidgetTester tester) async {
    final CookieJar jar = CookieJar();
    final ApiClient client = ApiClient(
      dio: ApiClient.buildDio(baseUrl: 'http://example.test', jar: jar)
        ..httpClientAdapter = _FailingAnnotations(),
      jar: jar,
    );

    await tester.pumpWidget(ProviderScope(
      overrides: <Override>[apiClientProvider.overrideWithValue(client)],
      child: const MaterialApp(home: ChapterPage(subjectId: 's1')),
    ));
    await tester.pumpAndSettle();

    expect(find.text('引言'), findsOneWidget, reason: '标注拉不到不该挡住阅读');
    expect(tester.takeException(), isNull);
  });
}

/// Same as [FakeBackend] but the annotation list always fails.
class _FailingAnnotations extends FakeBackend {
  @override
  Future<ResponseBody> fetch(
      RequestOptions options, Stream<dynamic>? stream, Future<void>? cancelFuture) async {
    if (options.path.endsWith('/annotations')) {
      return ResponseBody.fromString(
        '{"error":{"code":"internal","message":"读取标注失败"}}',
        500,
        headers: <String, List<String>>{
          Headers.contentTypeHeader: <String>[Headers.jsonContentType],
        },
      );
    }
    return super.fetch(options, stream, cancelFuture);
  }
}
