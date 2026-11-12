import 'dart:convert';

/// Version negotiated by the Flutter client and local host.
const localProtocolVersion = 1;

/// Categories of failures returned by the local client boundary.
enum LocalFailure {
  hostUnavailable,
  timeout,
  protocolMismatch,
  invalidResponse,
  api,
  closed,
}

/// Describes a failed local host request without exposing remote internals.
class LocalClientException implements Exception {
  /// Creates an exception with an optional sanitized API failure envelope.
  LocalClientException(
    this.kind, {
    this.error,
    this.requestId,
    this.httpStatus,
  });

  /// Broad local failure category.
  final LocalFailure kind;

  /// Sanitized API error returned by the host, when available.
  final LocalApiError? error;

  /// Request identifier associated with the failure, when available.
  final String? requestId;

  /// HTTP status associated with the failure, when available.
  final int? httpStatus;

  @override
  String toString() => 'LocalClientException(${kind.name})';
}

/// Indicates that a local API response violates the wire schema.
class LocalSchemaException implements Exception {
  @override
  String toString() => 'LocalSchemaException: Invalid local API response.';
}

/// Sanitized error payload returned by the local API.
class LocalApiError {
  LocalApiError._(this.code, this.message, this.retryable, this.details);

  /// Parses an API error object from JSON.
  factory LocalApiError.fromJson(Object? json) {
    final fields = _object(json, {'code', 'message', 'retryable', 'details'});
    final retryable = fields['retryable'];
    if (retryable is! bool) throw LocalSchemaException();
    return LocalApiError._(
      _text(fields['code']),
      _text(fields['message']),
      retryable,
      _snapshot(_object(fields['details'])),
    );
  }

  /// Stable machine-readable failure code.
  final String code;

  /// User-safe explanation supplied by the host.
  final String message;

  /// Whether retrying the request may succeed.
  final bool retryable;

  /// Structured, immutable diagnostic details.
  final Map<String, Object?> details;
}

/// Envelope shared by successful and failed local API responses.
class LocalEnvelope {
  LocalEnvelope._(this.requestId, this.data, this.error, this.meta);

  /// Decodes a JSON response body into a validated envelope.
  factory LocalEnvelope.decode(String text) {
    try {
      return LocalEnvelope.fromJson(jsonDecode(text));
    } on FormatException {
      throw LocalSchemaException();
    }
  }

  /// Parses and validates an envelope object.
  factory LocalEnvelope.fromJson(Object? json) {
    final fields = _object(json, {'request_id', 'data', 'error', 'meta'});
    final requestId = _uuid(fields['request_id']);
    final hasData = fields.containsKey('data');
    if (hasData == fields.containsKey('error')) throw LocalSchemaException();
    return LocalEnvelope._(
      requestId,
      hasData ? _snapshot(_object(fields['data'])) : null,
      hasData ? null : LocalApiError.fromJson(fields['error']),
      fields.containsKey('meta') ? _snapshot(_object(fields['meta'])) : null,
    );
  }

  /// UUID echoed by the host for request correlation.
  final String requestId;

  /// Immutable success payload, or null when the response is an error.
  final Map<String, Object?>? data;

  /// Sanitized error payload, or null when the response succeeded.
  final LocalApiError? error;

  /// Optional immutable response metadata.
  final Map<String, Object?>? meta;
}

/// Typed successful response returned by [LocalClient].
class LocalResponse<T> {
  /// Creates a typed response with optional immutable metadata.
  LocalResponse({required this.requestId, required this.data, this.meta});

  /// UUID echoed by the host for request correlation.
  final String requestId;

  /// Decoded response payload.
  final T data;

  /// Optional immutable response metadata.
  final Map<String, Object?>? meta;
}

/// Host lifecycle, monitor, coverage, and permission status.
class HostStatus {
  HostStatus._(
    this.protocolVersion,
    this.hostVersion,
    this.activeProfileId,
    this.monitorState,
    this.notificationPermission,
    this.coverageState,
  );

  /// Parses and validates a host status object.
  factory HostStatus.fromJson(Object? json) {
    final fields = _object(json, {
      'protocol_version',
      'host_version',
      'active_profile_id',
      'monitor_state',
      'notification_permission',
      'coverage',
    });
    final version = fields['protocol_version'];
    if (version is! int) throw LocalSchemaException();
    if (version != localProtocolVersion) {
      throw LocalClientException(LocalFailure.protocolMismatch);
    }
    return HostStatus._(
      version,
      _text(fields['host_version']),
      _uuid(fields['active_profile_id']),
      _text(fields['monitor_state']),
      _text(fields['notification_permission']),
      _text(_object(fields['coverage'], {'state'})['state']),
    );
  }

  /// Protocol version reported by the host.
  final int protocolVersion;

  /// Host build version.
  final String hostVersion;

  /// Active profile UUID.
  final String activeProfileId;

  /// Current monitoring lifecycle state.
  final String monitorState;

  /// Current native notification permission state.
  final String notificationPermission;

  /// Current refresh coverage state.
  final String coverageState;
}

/// Result of validating a rule definition.
class RuleValidation {
  RuleValidation._(this.summary, this.requiredFields);

  /// Parses a validated rule summary and required field list.
  factory RuleValidation.fromJson(Object? json) {
    final fields = _object(json, {'summary', 'required_fields'});
    final requiredFields = fields['required_fields'];
    if (requiredFields is! List) throw LocalSchemaException();
    return RuleValidation._(
      _text(fields['summary']),
      List<String>.unmodifiable(requiredFields.map(_text)),
    );
  }

  /// Human-readable rule summary.
  final String summary;

  /// Fields required to evaluate the rule.
  final List<String> requiredFields;
}

/// Wire-only device-flow fixture contract for the future account slice.
class DeviceSession {
  DeviceSession._(
    this.id,
    this.status,
    this.verificationUrl,
    this.userCode,
    this.expiresAt,
    this.pollIntervalSeconds,
  );

  /// Parses and validates a GitHub device-flow session.
  factory DeviceSession.fromJson(Object? json) {
    final fields = _object(json, {
      'id',
      'status',
      'verification_url',
      'user_code',
      'expires_at',
      'poll_interval_seconds',
    });
    final url = Uri.tryParse(_text(fields['verification_url']));
    if (url == null ||
        url.scheme != 'https' ||
        url.host != 'github.com' ||
        url.userInfo.isNotEmpty ||
        url.port != 443 ||
        url.path != '/login/device' ||
        url.hasQuery ||
        url.hasFragment) {
      throw LocalSchemaException();
    }
    final interval = fields['poll_interval_seconds'];
    if (interval is! int || interval <= 0) throw LocalSchemaException();
    return DeviceSession._(
      _uuid(fields['id']),
      _text(fields['status']),
      url,
      _text(fields['user_code']),
      _utc(fields['expires_at']),
      interval,
    );
  }

  /// Device-flow session UUID.
  final String id;

  /// Current device-flow state.
  final String status;

  /// GitHub verification URL.
  final Uri verificationUrl;

  /// User code entered at the verification URL.
  final String userCode;

  /// UTC instant at which the session expires.
  final DateTime expiresAt;

  /// Minimum polling interval requested by GitHub.
  final int pollIntervalSeconds;
}

Map<String, Object?> _object(Object? value, [Set<String>? allowed]) {
  if (value is! Map<String, dynamic> ||
      (allowed != null && value.keys.any((key) => !allowed.contains(key)))) {
    throw LocalSchemaException();
  }
  return value;
}

String _text(Object? value) {
  if (value is! String || value.isEmpty) throw LocalSchemaException();
  return value;
}

final _uuidPattern = RegExp(
  r'^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$',
);

String _uuid(Object? value) {
  final text = _text(value);
  if (!_uuidPattern.hasMatch(text) ||
      text == '00000000-0000-0000-0000-000000000000') {
    throw LocalSchemaException();
  }
  return text;
}

final _utcPattern = RegExp(
  r'^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d{1,9})?(?:Z|[+-]00:00)$',
);

DateTime _utc(Object? value) {
  final text = _text(value);
  final match = _utcPattern.firstMatch(text);
  final instant = DateTime.tryParse(text);
  if (match == null || instant == null || !instant.isUtc) {
    throw LocalSchemaException();
  }
  // DateTime.parse normalizes invalid days and hours instead of rejecting them.
  final components = [
    instant.year,
    instant.month,
    instant.day,
    instant.hour,
    instant.minute,
    instant.second,
  ];
  for (var index = 0; index < components.length; index++) {
    if (components[index] != int.parse(match.group(index + 1)!)) {
      throw LocalSchemaException();
    }
  }
  return instant;
}

Map<String, Object?> _snapshot(Map<String, Object?> value) =>
    Map<String, Object?>.unmodifiable(
      value.map((key, item) => MapEntry(key, _freeze(item))),
    );

Object? _freeze(Object? value) {
  if (value is Map<String, dynamic>) return _snapshot(value);
  if (value is List) return List<Object?>.unmodifiable(value.map(_freeze));
  if (value == null ||
      value is String ||
      value is bool ||
      (value is num && value.isFinite)) {
    return value;
  }
  throw LocalSchemaException();
}
