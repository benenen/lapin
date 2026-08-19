import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'app.dart';
import 'core/network/api_client.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  // The cookie jar needs a directory on disk, so the client is built before the
  // first frame and injected — nothing else in the tree constructs one.
  final ApiClient client = await ApiClient.create();
  runApp(
    ProviderScope(
      overrides: <Override>[apiClientProvider.overrideWithValue(client)],
      child: const LapinApp(),
    ),
  );
}
