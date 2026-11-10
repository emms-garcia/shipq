import 'package:flutter/material.dart';

void main() {
  runApp(const ShipqApp());
}

class ShipqApp extends StatelessWidget {
  const ShipqApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Shipq',
      theme: ThemeData(
        colorScheme: ColorScheme.fromSeed(seedColor: Colors.indigo),
      ),
      home: Scaffold(
        appBar: AppBar(title: const Text('Shipq')),
        body: Center(
          child: SingleChildScrollView(
            padding: const EdgeInsets.all(24),
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 560),
              child: const Column(
                mainAxisSize: MainAxisSize.min,
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text('Your pull requests, organized locally.'),
                  SizedBox(height: 16),
                  Text(
                    'GitHub connection and monitoring are not implemented yet.',
                  ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}
