/// Build-time configuration.
///
/// Pass the backend origin at run/build time rather than hard-coding it:
///
/// ```sh
/// flutter run --dart-define=API_BASE_URL=http://10.0.2.2:8080
/// ```
///
/// The default targets the Android emulator, where `10.0.2.2` is the host
/// machine — `localhost` inside the emulator is the emulator itself.
class Env {
  const Env._();

  static const String apiBaseUrl = String.fromEnvironment(
    'API_BASE_URL',
    defaultValue: 'http://10.0.2.2:8080',
  );
}
