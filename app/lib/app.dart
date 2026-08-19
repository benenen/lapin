import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'core/router/app_router.dart';

class LapinApp extends ConsumerWidget {
  const LapinApp({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) => MaterialApp.router(
        title: 'Lapin',
        debugShowCheckedModeBanner: false,
        routerConfig: ref.watch(routerProvider),
        theme: ThemeData(
          colorScheme: ColorScheme.fromSeed(seedColor: const Color(0xFF33483E)),
          useMaterial3: true,
        ),
      );
}
