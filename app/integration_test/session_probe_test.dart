import 'package:cookie_jar/cookie_jar.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';
import 'package:lapin_app/core/network/api_client.dart';

/// Network-layer probe that runs on the device, with no widgets involved.
///
/// The same handshake passes on the host against an in-memory jar, so this
/// isolates what is different on a device: the persistent cookie jar and the
/// emulator's route to the host.
void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets('设备上：登录后的 cookie 能带到后续请求', (WidgetTester tester) async {
    final ApiClient client = await ApiClient.create();
    await client.clearSession();

    final Map<String, dynamic> login = await client.post<Map<String, dynamic>>(
      '/api/v1/auth/login',
      body: <String, String>{'email': 'admin@localhost', 'password': 'admin12345678'},
    );
    debugPrint('PROBE login user=${(login['user'] as Map<String, dynamic>?)?['email']}');

    final List<Cookie> stored = await client.jar.loadForRequest(Uri.parse(client.dio.options.baseUrl));
    debugPrint('PROBE 罐中 cookie=${stored.map((Cookie c) => c.name).toList()}');
    expect(
      stored.map((Cookie cookie) => cookie.name),
      containsAll(<String>['lapin_session', 'lapin_csrf']),
    );

    final Map<String, dynamic> me = await client.get<Map<String, dynamic>>('/api/v1/me');
    debugPrint('PROBE /me=${me['email']}');

    final List<dynamic> subjects = await client.get<List<dynamic>>('/api/v1/subjects');
    debugPrint('PROBE 课程数=${subjects.length}');
    expect(subjects, isNotEmpty);
  });
}
