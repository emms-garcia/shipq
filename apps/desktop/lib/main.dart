import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import 'application/local_client.dart';
import 'application/local_models.dart';

void main() => runApp(const ShipqApp());

class HostUnavailable implements Exception {
  const HostUnavailable();
}

abstract interface class HostConnector {
  Future<String> socketPath();
  Future<void> startHost();
  Future<void> stopHost();
  Future<String> serviceStatus();
  Future<String> registerService();
  Future<String> unregisterService();
  Future<HostStatus> status(String socketPath);
}

class PlatformHostConnector implements HostConnector {
  const PlatformHostConnector();
  static const _channel = MethodChannel('com.shipq.desktop/platform');
  static const _fixtureSocket = String.fromEnvironment('SHIPQ_SOCKET_PATH');

  @override
  Future<String> socketPath() async {
    if (_fixtureSocket.isNotEmpty) return _fixtureSocket;
    final path = await _channel.invokeMethod<String>('socketPath');
    if (path == null || !path.startsWith('/')) throw const HostUnavailable();
    return path;
  }

  @override
  Future<void> startHost() async {
    if (_fixtureSocket.isEmpty) await _channel.invokeMethod<void>('startHost');
  }

  @override
  Future<void> stopHost() async {
    if (_fixtureSocket.isEmpty) await _channel.invokeMethod<void>('stopHost');
  }

  @override
  Future<String> serviceStatus() async => _fixtureSocket.isNotEmpty
      ? 'unavailable'
      : await _channel.invokeMethod<String>('serviceStatus') ?? 'unavailable';

  @override
  Future<String> registerService() async =>
      await _channel.invokeMethod<String>('registerService') ?? 'unavailable';

  @override
  Future<String> unregisterService() async =>
      await _channel.invokeMethod<String>('unregisterService') ?? 'unavailable';

  @override
  Future<HostStatus> status(String socketPath) async {
    final client = LocalClient(socketPath: socketPath);
    try {
      return (await client.getStatus()).data;
    } on LocalClientException {
      throw const HostUnavailable();
    } finally {
      client.close();
    }
  }
}

class ShipqApp extends StatelessWidget {
  const ShipqApp({super.key, this.connector = const PlatformHostConnector()});
  final HostConnector connector;

  @override
  Widget build(BuildContext context) => MaterialApp(
    title: 'Shipq',
    theme: ThemeData(
      colorScheme: ColorScheme.fromSeed(seedColor: Colors.indigo),
    ),
    home: _FoundationHome(connector: connector),
  );
}

class _FoundationHome extends StatefulWidget {
  const _FoundationHome({required this.connector});
  final HostConnector connector;

  @override
  State<_FoundationHome> createState() => _FoundationHomeState();
}

class _FoundationHomeState extends State<_FoundationHome> {
  HostStatus? _status;
  String? _socketPath;
  bool _busy = false;
  String? _error;
  String _serviceState = 'unavailable';

  @override
  void initState() {
    super.initState();
    _refresh();
  }

  Future<void> _refresh() async {
    try {
      final serviceState = await widget.connector.serviceStatus();
      if (mounted) setState(() => _serviceState = serviceState);
    } on Object {
      if (mounted) setState(() => _serviceState = 'unavailable');
    }
    try {
      final path = _socketPath ?? await widget.connector.socketPath();
      final status = await widget.connector.status(path);
      if (mounted) {
        setState(() {
          _socketPath = path;
          _status = status;
          _error = null;
        });
      }
    } on Object {
      if (mounted) {
        setState(() {
          _status = null;
          _error = 'Local service is stopped.';
        });
      }
    }
  }

  Future<void> _toggleLoginService() async {
    setState(() => _busy = true);
    try {
      final enabled =
          _serviceState == 'enabled' || _serviceState == 'requires_approval';
      final state = enabled
          ? await widget.connector.unregisterService()
          : await widget.connector.registerService();
      if (mounted) setState(() => _serviceState = state);
      await _refresh();
    } on Object {
      if (mounted) {
        setState(() => _error = 'Unable to change login service approval.');
      }
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _start() async {
    setState(() => _busy = true);
    try {
      await widget.connector.startHost();
      await _refresh();
    } on Object {
      if (mounted) {
        setState(() => _error = 'Unable to start the local service.');
      }
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _stop() async {
    setState(() => _busy = true);
    try {
      await widget.connector.stopHost();
      if (mounted) {
        setState(() {
          _status = null;
          _error = 'Local service is stopped.';
        });
      }
    } on Object {
      if (mounted) setState(() => _error = 'Unable to stop the local service.');
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final status = _status;
    return Scaffold(
      appBar: AppBar(title: const Text('Shipq')),
      body: Center(
        child: SingleChildScrollView(
          padding: const EdgeInsets.all(24),
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 560),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const Text('Your pull requests, organized locally.'),
                const SizedBox(height: 16),
                const Text(
                  'GitHub connection and monitoring are not implemented yet.',
                ),
                const SizedBox(height: 24),
                Text(
                  status == null
                      ? (_error ?? 'Checking the local service…')
                      : 'Connected to host ${status.hostVersion}. '
                            'Notification permission: ${status.notificationPermission}.',
                ),
                const SizedBox(height: 12),
                FilledButton(
                  onPressed: _busy ? null : (status == null ? _start : _stop),
                  child: Text(
                    status == null
                        ? 'Start local service'
                        : 'Stop local service',
                  ),
                ),
                const SizedBox(height: 8),
                Text('Login service: $_serviceState'),
                TextButton(
                  onPressed: _busy ? null : _toggleLoginService,
                  child: Text(
                    _serviceState == 'enabled' ||
                            _serviceState == 'requires_approval'
                        ? 'Disable login service'
                        : 'Enable login service',
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
