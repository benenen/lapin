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
  fail('等待超时，未出现：${finder.description}');
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
    await pumpUntil(tester, find.text('我的课程'));
    expect(find.byType(ListTile), findsWidgets, reason: '登录成功后应至少列出一个课程');

    // 3. 打开课程，等章节正文渲染出来
    await tester.tap(find.byType(ListTile).first);
    await settle(tester, seconds: 3);
    await pumpUntil(tester, find.byType(Drawer).evaluate().isEmpty ? find.byType(Scaffold) : find.byType(Scaffold));
    await settle(tester, seconds: 8);

    // Markdown 渲染出来的正文里应当有实际文字，而不是空白或错误页
    expect(find.textContaining('暂无正文'), findsNothing);
    expect(find.byType(CircularProgressIndicator), findsNothing, reason: '正文应已加载完成');
    final Iterable<Text> texts = tester.widgetList<Text>(find.byType(Text));
    final int bodyCharacters = texts
        .map((Text text) => text.data?.length ?? 0)
        .fold<int>(0, (int sum, int length) => sum + length);
    expect(bodyCharacters, greaterThan(200), reason: '章节正文应当渲染出可观的文字量');
    debugPrint('E2E_OK 渲染字符数=$bodyCharacters');
  });
}
