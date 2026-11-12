import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'local_models.dart';

/// Sends typed requests to the host over its private Unix-socket HTTP API.
class LocalClient {
  /// Creates a client for an absolute Unix socket with a per-request deadline.
  LocalClient({
    required this.socketPath,
    this.timeout = const Duration(seconds: 10),
  }) {
    if (!socketPath.startsWith('/') ||
        socketPath.contains('\u0000') ||
        utf8.encode(socketPath).length > _maxSocketPathBytes ||
        timeout <= Duration.zero) {
      throw ArgumentError(
        'Use a short absolute Unix socket path and positive timeout.',
      );
    }
  }

  static const _maxSocketPathBytes = 103;
  static const _maxBodyBytes = 1024 * 1024;

  /// Absolute path of the host's Unix socket.
  final String socketPath;

  /// Maximum time allowed for each request.
  final Duration timeout;
  final _active = <HttpClient>{};
  bool _closed = false;

  /// Retrieves the host's current lifecycle and permission status.
  Future<LocalResponse<HostStatus>> getStatus() =>
      _request('GET', '/v1/status', HostStatus.fromJson);

  /// Validates a rule definition without persisting it.
  Future<LocalResponse<RuleValidation>> validateRule(
    Map<String, Object?> definition,
  ) => _request(
    'POST',
    '/v1/rules/validate',
    RuleValidation.fromJson,
    body: definition,
  );

  /// Cancels active requests and prevents new requests from starting.
  void close() {
    _closed = true;
    for (final client in _active) {
      client.close(force: true);
    }
  }

  Future<LocalResponse<T>> _request<T>(
    String method,
    String path,
    T Function(Object?) decode, {
    Map<String, Object?>? body,
  }) async {
    if (_closed) throw LocalClientException(LocalFailure.closed);
    List<int>? bytes;
    if (body != null) {
      try {
        bytes = utf8.encode(jsonEncode(body));
      } on JsonUnsupportedObjectError {
        throw ArgumentError('Rule definition must be JSON encodable.');
      }
      if (bytes.length > _maxBodyBytes) {
        throw ArgumentError('Local request body exceeds the protocol limit.');
      }
    }
    // Per-request ownership makes timeout/close cancel I/O without aborting peers.
    final client = HttpClient()
      ..autoUncompress = false
      ..findProxy = ((_) => 'DIRECT')
      ..connectionFactory = (uri, proxyHost, proxyPort) => Socket.startConnect(
        InternetAddress(socketPath, type: InternetAddressType.unix),
        0,
      );
    _active.add(client);
    try {
      return await _exchange(
        client,
        method,
        path,
        decode,
        bytes,
      ).timeout(timeout);
    } on TimeoutException {
      throw LocalClientException(
        _closed ? LocalFailure.closed : LocalFailure.timeout,
      );
    } on SocketException {
      throw LocalClientException(
        _closed ? LocalFailure.closed : LocalFailure.hostUnavailable,
      );
    } on HttpException {
      throw LocalClientException(
        _closed ? LocalFailure.closed : LocalFailure.hostUnavailable,
      );
    } on FormatException {
      throw LocalClientException(LocalFailure.invalidResponse);
    } on LocalSchemaException {
      throw LocalClientException(LocalFailure.invalidResponse);
    } finally {
      _active.remove(client);
      client.close(force: true);
    }
  }

  Future<LocalResponse<T>> _exchange<T>(
    HttpClient client,
    String method,
    String path,
    T Function(Object?) decode,
    List<int>? bytes,
  ) async {
    final request = await client.openUrl(
      method,
      Uri(scheme: 'http', host: 'shipq.local', path: path),
    );
    request.followRedirects = false;
    request.headers.set(HttpHeaders.acceptHeader, 'application/json');
    if (bytes != null) {
      request.headers.contentType = ContentType.json;
      request.contentLength = bytes.length;
      request.add(bytes);
    }
    final response = await request.close();
    final media = ContentType.parse(
      _header(response, HttpHeaders.contentTypeHeader) ?? '',
    );
    final charset = media.parameters['charset'];
    if (media.mimeType != 'application/json' ||
        (charset != null && charset.toLowerCase() != 'utf-8') ||
        media.parameters.keys.any((key) => key != 'charset') ||
        response.headers[HttpHeaders.contentEncodingHeader] != null ||
        response.contentLength > _maxBodyBytes) {
      throw LocalClientException(LocalFailure.invalidResponse);
    }
    if (_header(response, 'X-Shipq-Protocol-Version') !=
        '$localProtocolVersion') {
      throw LocalClientException(LocalFailure.protocolMismatch);
    }
    final buffer = BytesBuilder(copy: false);
    await for (final chunk in response) {
      if (buffer.length + chunk.length > _maxBodyBytes) {
        throw LocalClientException(LocalFailure.invalidResponse);
      }
      buffer.add(chunk);
    }
    final envelope = LocalEnvelope.decode(utf8.decode(buffer.takeBytes()));
    if (_header(response, 'X-Request-ID') != envelope.requestId) {
      throw LocalClientException(LocalFailure.invalidResponse);
    }
    final success = response.statusCode >= 200 && response.statusCode < 300;
    if (success != (envelope.data != null)) {
      throw LocalClientException(LocalFailure.invalidResponse);
    }
    if (envelope.error != null) {
      throw LocalClientException(
        LocalFailure.api,
        error: envelope.error,
        requestId: envelope.requestId,
        httpStatus: response.statusCode,
      );
    }
    return LocalResponse(
      requestId: envelope.requestId,
      data: decode(envelope.data),
      meta: envelope.meta,
    );
  }

  String? _header(HttpClientResponse response, String name) {
    final values = response.headers[name];
    return values != null && values.length == 1 ? values.single : null;
  }
}
