import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shipq/main.dart' as app;

void main() {
  for (final size in [const Size(320, 240), const Size(1280, 800)]) {
    testWidgets('scaffold explains unavailable features at $size', (
      tester,
    ) async {
      tester.view.devicePixelRatio = 1;
      tester.view.physicalSize = size;
      addTearDown(tester.view.resetDevicePixelRatio);
      addTearDown(tester.view.resetPhysicalSize);

      app.main();
      await tester.pumpAndSettle();

      expect(tester.takeException(), isNull);
      expect(
        find.text('GitHub connection and monitoring are not implemented yet.'),
        findsOneWidget,
      );
      expect(find.byType(FloatingActionButton), findsNothing);
    });
  }
}
