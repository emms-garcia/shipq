import 'dart:convert';

const localProtocolVersion = 1;

enum LocalFailure {
  hostUnavailable,
  timeout,
  protocolMismatch,
  invalidResponse,
  api,
  closed,
}

class LocalClientException implements Exception {
  LocalClientException(
    this.kind, {
    this.error,
    this.requestId,
    this.httpStatus,
  });

  final LocalFailure kind;
  final LocalApiError? error;
  final String? requestId;
  final int? httpStatus;

  @override
  String toString() => 'LocalClientException(${kind.name})';
}

class LocalSchemaException implements Exception {
  @override
  String toString() => 'LocalSchemaException: Invalid local API response.';
}

class LocalApiError {
  LocalApiError._(this.code, this.message, this.retryable, this.details);

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

  final String code;
  final String message;
  final bool retryable;
  final Map<String, Object?> details;
}

class LocalEnvelope {
  LocalEnvelope._(this.requestId, this.data, this.error, this.meta);

  factory LocalEnvelope.decode(String text) {
    try {
      return LocalEnvelope.fromJson(jsonDecode(text));
    } on FormatException {
      throw LocalSchemaException();
    }
  }

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

  final String requestId;
  final Map<String, Object?>? data;
  final LocalApiError? error;
  final Map<String, Object?>? meta;
}

class LocalResponse<T> {
  LocalResponse({required this.requestId, required this.data, this.meta});

  final String requestId;
  final T data;
  final Map<String, Object?>? meta;
}

class HostStatus {
  HostStatus._(
    this.protocolVersion,
    this.hostVersion,
    this.activeProfileId,
    this.monitorState,
    this.notificationPermission,
    this.coverageState,
  );

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

  final int protocolVersion;
  final String hostVersion;
  final String activeProfileId;
  final String monitorState;
  final String notificationPermission;
  final String coverageState;
}

class RuleValidation {
  RuleValidation._(this.summary, this.requiredFields);

  factory RuleValidation.fromJson(Object? json) {
    final fields = _object(json, {'summary', 'required_fields'});
    final requiredFields = fields['required_fields'];
    if (requiredFields is! List) throw LocalSchemaException();
    return RuleValidation._(
      _text(fields['summary']),
      List<String>.unmodifiable(requiredFields.map(_text)),
    );
  }

  final String summary;
  final List<String> requiredFields;
}

// A wire-only fixture contract; device-flow routes arrive with the account slice.
class DeviceSession {
  DeviceSession._(
    this.id,
    this.status,
    this.verificationUrl,
    this.userCode,
    this.expiresAt,
    this.pollIntervalSeconds,
  );

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

  final String id;
  final String status;
  final Uri verificationUrl;
  final String userCode;
  final DateTime expiresAt;
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
