import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';
import 'package:lapin_app/app.dart';
import 'package:lapin_app/core/network/api_client.dart';

/// Drives the real app against a real Lapin backend on a device.
///
/// ```sh
/// flutter test integration_test/reading_flow_test.dart -d emulator-5554 \
///   --dart-define=API_BASE_URL=http://10.0.2.2:8080 \
///   --dart-define=E2E_EMAIL=admin@localhost \
///   --dart-define=E2E_PASSWORD=admin12345678
/// ```
///
/// `10.0.2.2` is how the Android emulator reaches the host's loopback, which is
/// where `make watch` serves.
const String email = String.fromEnvironment('E2E_EMAIL', defaultValue: 'admin@localhost');
const String password = String.fromEnvironment('E2E_PASSWORD', defaultValue: 'admin12345678');

/// Keeps the app on screen at the end of the run so a screenshot can be taken.
const int holdSeconds = int.fromEnvironment('E2E_HOLD_SECONDS');

Future<void> settle(WidgetTester tester, {int seconds = 20}) async {
  // pumpAndSettle would time out against a live server, so poll instead.
  final DateTime deadline = DateTime.now().add(Duration(seconds: seconds));
  while (DateTime.now().isBefore(deadline)) {
    await tester.pump(const Duration(milliseconds: 200));
  }
}

Future<void> pumpUntil(WidgetTester tester, Finder finder, {int seconds = 30}) async {
  final DateTime deadline = DateTime.now().add(Duration(seconds: seconds));
  while (DateTime.now().isBefore(deadline)) {
    await tester.pump(const Duration(milliseconds: 200));
    if (finder.evaluate().isNotEmpty) return;
  }
  fail('等待超时，未出现：$finder');
}

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets('登录 → 课程列表 → 章节正文', (WidgetTester tester) async {
    final ApiClient client = await ApiClient.create();
    // Start from a clean session so the run always exercises the sign-in path.
    await client.clearSession();

    await tester.pumpWidget(
      ProviderScope(
        overrides: <Override>[apiClientProvider.overrideWithValue(client)],
        child: const LapinApp(),
      ),
    );

    // 1. 登录页
    await pumpUntil(tester, find.text('登录'));
    expect(find.text('Lapin'), findsOneWidget);

    await tester.enterText(find.byType(TextFormField).first, email);
    await tester.enterText(find.byType(TextFormField).last, password);
    await tester.pump();
    await tester.tap(find.widgetWithText(FilledButton, '登录'));

    // 2. 课程列表
    // 「我的课程」是 AppBar 标题，列表还在加载时它就已经渲染了，所以要等到列表项本身。
    await pumpUntil(tester, find.text('我的课程'));
    await pumpUntil(tester, find.byType(ListTile));
    expect(find.byType(ListTile), findsWidgets, reason: '登录成功后应至少列出一个课程');

    // 3. 打开课程，等章节正文渲染出来
    await tester.tap(find.byType(ListTile).first);
    await settle(tester, seconds: 2);
    // 章节页拉完 subject 详情后，AppBar 标题会变成章节名，返回按钮就位
    await pumpUntil(tester, find.byIcon(Icons.arrow_back));
    await settle(tester, seconds: 6);

    // Markdown 渲染出来的正文里应当有实际文字，而不是空白或错误页
    expect(find.textContaining('暂无正文'), findsNothing);
    expect(find.byType(CircularProgressIndicator), findsNothing, reason: '正文应已加载完成');
    // Counting Text widgets is not enough. Markdown renders spans, and with
    // selectable: true the body lives in EditableText, whose text a Text or
    // RichText finder never sees. Sum both surfaces.
    final int spanCharacters = tester
        .widgetList<RichText>(find.byType(RichText))
        .map((RichText text) => text.text.toPlainText().length)
        .fold<int>(0, (int sum, int length) => sum + length);
    final int selectableCharacters = tester
        .widgetList<EditableText>(find.byType(EditableText))
        .map((EditableText text) => text.controller.text.length)
        .fold<int>(0, (int sum, int length) => sum + length);
    final int bodyCharacters = spanCharacters + selectableCharacters;
    expect(bodyCharacters, greaterThan(200), reason: '章节正文应当渲染出可观的文字量');
    debugPrint('E2E_OK 渲染字符数=$bodyCharacters');

    // 4. 标注入口：本章有标注就列出来，没有就给出引导，两种都算通过。
    await tester.tap(find.byIcon(Icons.comment_outlined));
    await settle(tester, seconds: 2);
    final bool listed = find.byType(ListTile).evaluate().isNotEmpty;
    final bool empty = find.textContaining('还没有标注').evaluate().isNotEmpty;
    expect(listed || empty, isTrue, reason: '标注面板应当打开');
    debugPrint('E2E_OK 标注面板 已有标注=$listed');
    if (listed) {
      await tester.tap(find.byType(ListTile).first);
      await settle(tester, seconds: 2);
      expect(find.textContaining('·'), findsWidgets, reason: '详情应显示作者与时间');
    }
    // 留出窗口给外部截图核对高亮渲染。
    await settle(tester, seconds: holdSeconds);
  });
}
