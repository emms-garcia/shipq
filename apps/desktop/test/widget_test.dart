import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shipq/application/local_models.dart';
import 'package:shipq/main.dart' as app;

class FakeHostConnector implements app.HostConnector {
  FakeHostConnector({this.running = false});

  bool running;
  String loginService = 'not_registered';

  @override
  Future<String> socketPath() async => '/tmp/shipq-test/api.sock';

  @override
  Future<void> startHost() async => running = true;

  @override
  Future<void> stopHost() async => running = false;

  @override
  Future<String> serviceStatus() async => loginService;

  @override
  Future<String> registerService() async => loginService = 'enabled';

  @override
  Future<String> unregisterService() async => loginService = 'not_registered';

  @override
  Future<HostStatus> status(String socketPath) async {
    if (!running) throw const app.HostUnavailable();
    return HostStatus.fromJson({
      'protocol_version': 1,
      'host_version': 'test',
      'active_profile_id': '22222222-2222-4222-8222-222222222222',
      'monitor_state': 'unavailable',
      'notification_permission': 'unavailable',
      'coverage': {'state': 'unknown'},
    });
  }
}

void main() {
  for (final size in [const Size(320, 240), const Size(1280, 800)]) {
    testWidgets('scaffold explains unavailable features at $size', (
      tester,
    ) async {
      tester.view.devicePixelRatio = 1;
      tester.view.physicalSize = size;
      addTearDown(tester.view.resetDevicePixelRatio);
      addTearDown(tester.view.resetPhysicalSize);

      await tester.pumpWidget(app.ShipqApp(connector: FakeHostConnector()));
      await tester.pumpAndSettle();

      expect(tester.takeException(), isNull);
      expect(
        find.text('GitHub connection and monitoring are not implemented yet.'),
        findsOneWidget,
      );
      expect(find.byType(FloatingActionButton), findsNothing);
      expect(find.text('Start local service'), findsOneWidget);
    });
  }

  testWidgets('starts the local host only after an explicit action', (
    tester,
  ) async {
    final connector = FakeHostConnector();
    await tester.pumpWidget(app.ShipqApp(connector: connector));
    await tester.pumpAndSettle();

    expect(find.text('Local service is stopped.'), findsOneWidget);
    await tester.tap(find.text('Start local service'));
    await tester.pumpAndSettle();

    expect(connector.running, isTrue);
    expect(find.textContaining('Connected to host test'), findsOneWidget);
    expect(find.text('Stop local service'), findsOneWidget);
  });
}
