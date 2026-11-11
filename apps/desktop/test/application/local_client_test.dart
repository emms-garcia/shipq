import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:shipq/application/local_client.dart';
import 'package:shipq/application/local_models.dart';

const requestId = 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa';
final fixtures =
    jsonDecode(
          File(
            '../../tests/fixtures/protocol/base_envelopes.json',
          ).readAsStringSync(),
        )
        as Map<String, dynamic>;

Map<String, dynamic> fixture(String id) => (fixtures['valid'] as List)
    .cast<Map<String, dynamic>>()
    .firstWhere((item) => item['id'] == id);

Map<String, dynamic> statusEnvelope() =>
    jsonDecode(
          jsonEncode(
            fixture('host-status-unavailable-capabilities')['envelope'],
          ),
        )
        as Map<String, dynamic>;

TypeMatcher<LocalClientException> failure(LocalFailure kind) =>
    isA<LocalClientException>().having((error) => error.kind, 'kind', kind);

Future<String> serve(Future<void> Function(HttpRequest) handle) async {
  final directory = await Directory('/tmp').createTemp('sqc-');
  final path = '${directory.path}/api.sock';
  final server = await HttpServer.bind(
    InternetAddress(path, type: InternetAddressType.unix),
    0,
  );
  server.listen(handle);
  addTearDown(() async {
    await server.close(force: true);
    await directory.delete(recursive: true);
  });
  return path;
}

Future<void> reply(
  HttpRequest request,
  Map<String, dynamic> envelope, {
  int status = 200,
  String? protocol = '1',
  String? correlation,
  String contentType = 'application/json; charset=utf-8',
}) async {
  request.response.statusCode = status;
  request.response.headers.set(HttpHeaders.contentTypeHeader, contentType);
  if (protocol != null) {
    request.response.headers.set('X-Shipq-Protocol-Version', protocol);
  }
  request.response.headers.set(
    'X-Request-ID',
    correlation ?? envelope['request_id'] as String,
  );
  request.response.add(utf8.encode(jsonEncode(envelope)));
  await request.response.close();
}

void main() {
  group('shared envelopes', () {
    for (final item
        in (fixtures['valid'] as List).cast<Map<String, dynamic>>()) {
      test(item['id'] as String, () {
        final envelope = LocalEnvelope.decode(jsonEncode(item['envelope']));
        expect(envelope.requestId, item['envelope']['request_id']);
        if (item['envelope'].containsKey('error')) {
          final error = envelope.error!;
          expect(error.code, item['envelope']['error']['code']);
          expect(error.retryable, item['envelope']['error']['retryable']);
          expect(envelope.data, isNull);
        } else {
          expect(envelope.data, item['envelope']['data']);
          expect(envelope.error, isNull);
        }
      });
    }
    for (final item
        in (fixtures['invalid'] as List).cast<Map<String, dynamic>>()) {
      test(item['id'] as String, () {
        expect(
          () => LocalEnvelope.fromJson(item['envelope']),
          throwsA(isA<LocalSchemaException>()),
        );
      });
    }
    test('an empty partial query remains an empty partial query', () {
      final envelope = LocalEnvelope.fromJson(
        fixture('empty-partial-query')['envelope'],
      );
      expect(envelope.data!['items'], isEmpty);
      expect(envelope.meta!['freshness'], 'stale');
      expect((envelope.meta!['coverage'] as Map)['state'], 'partial');
      expect((envelope.meta!['coverage'] as Map)['unknown_pr_count'], 1);
    });
    test('payload and metadata are immutable snapshots', () {
      final source =
          jsonDecode(jsonEncode(fixture('empty-partial-query')['envelope']))
              as Map<String, dynamic>;
      final envelope = LocalEnvelope.fromJson(source);
      (source['data']['items'] as List).add({'title': 'later'});
      source['meta']['coverage']['state'] = 'complete';
      expect(envelope.data!['items'], isEmpty);
      expect((envelope.meta!['coverage'] as Map)['state'], 'partial');
      expect(
        () => (envelope.data!['items'] as List).add({}),
        throwsUnsupportedError,
      );
      expect(
        () => (envelope.meta!['coverage'] as Map)['state'] = 'complete',
        throwsUnsupportedError,
      );
    });
    for (final invalid in <Object?>[
      null,
      [],
      {'request_id': requestId},
      {'request_id': requestId, 'data': null},
      {'request_id': requestId, 'data': []},
      {'request_id': 1, 'data': {}},
      {'request_id': '00000000-0000-0000-0000-000000000000', 'data': {}},
      {'request_id': requestId.toUpperCase(), 'data': {}},
      {'request_id': requestId, 'data': {}, 'meta': null},
      {'request_id': requestId, 'data': {}, 'extra': true},
      {
        'request_id': requestId,
        'error': {
          'code': 'internal',
          'message': 'failure',
          'retryable': 'false',
          'details': {},
        },
      },
      {
        'request_id': requestId,
        'error': {
          'code': '',
          'message': 'failure',
          'retryable': false,
          'details': {},
        },
      },
      {
        'request_id': requestId,
        'error': {
          'code': 'internal',
          'message': 'failure',
          'retryable': false,
          'details': [],
        },
      },
      {
        'request_id': requestId,
        'error': {'code': 'internal', 'message': 'failure', 'retryable': false},
      },
    ].indexed) {
      test('rejects invalid envelope shape ${invalid.$1}', () {
        expect(
          () => LocalEnvelope.fromJson(invalid.$2),
          throwsA(isA<LocalSchemaException>()),
        );
      });
    }
    test('malformed JSON errors do not expose response contents', () {
      try {
        LocalEnvelope.decode('{"data":"SYNTHETIC-PRIVATE"');
        fail('expected schema failure');
      } on LocalSchemaException catch (error) {
        expect(error.toString(), isNot(contains('SYNTHETIC-PRIVATE')));
      }
    });
  });

  group('typed payloads', () {
    test('status preserves unavailable capabilities and unknown coverage', () {
      final status = HostStatus.fromJson(statusEnvelope()['data']);
      expect(status.protocolVersion, 1);
      expect(status.hostVersion, '0.0.0-dev');
      expect(status.activeProfileId, 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb');
      expect(status.monitorState, 'unavailable');
      expect(status.notificationPermission, 'unavailable');
      expect(status.coverageState, 'unknown');
    });
    for (final change in <Map<String, dynamic>>[
      {'protocol_version': '1'},
      {'active_profile_id': null},
      {'active_profile_id': 'not-a-uuid'},
      {'host_version': ''},
      {'monitor_state': false},
      {'coverage': {}},
      {
        'coverage': {'state': null},
      },
      {'extra': true},
    ].indexed) {
      test('status rejects incompatible fields ${change.$1}', () {
        final data = statusEnvelope()['data'] as Map<String, dynamic>;
        data.addAll(change.$2);
        expect(
          () => HostStatus.fromJson(data),
          throwsA(isA<LocalSchemaException>()),
        );
      });
    }
    test('status protocol version is independent of rule format version', () {
      final data = statusEnvelope()['data'] as Map<String, dynamic>;
      data['protocol_version'] = 2;
      expect(
        () => HostStatus.fromJson(data),
        throwsA(failure(LocalFailure.protocolMismatch)),
      );
    });
    test(
      'validation displays core summary and dependencies without evaluation',
      () {
        final validation = RuleValidation.fromJson(
          fixture('rule-validation-summary')['envelope']['data'],
        );
        expect(validation.summary, 'approval count is at least 3');
        expect(validation.requiredFields, ['approval_count']);
        expect(
          () => validation.requiredFields.add('author'),
          throwsUnsupportedError,
        );
        expect(
          () => RuleValidation.fromJson({
            'summary': 'x',
            'required_fields': [null],
          }),
          throwsA(isA<LocalSchemaException>()),
        );
      },
    );
    test('device session exposes only user-facing authorization guidance', () {
      final session = DeviceSession.fromJson(
        fixture('device-session-pending')['envelope']['data'],
      );
      expect(session.id, '44444444-4444-4444-8444-444444444444');
      expect(session.status, 'pending');
      expect(
        session.verificationUrl,
        Uri.parse('https://github.com/login/device'),
      );
      expect(session.userCode, 'FIXT-0000');
      expect(session.expiresAt, DateTime.utc(2026, 1, 2, 10, 15));
      expect(session.pollIntervalSeconds, 5);
    });
    for (final change in <Map<String, dynamic>>[
      {'access_token': 'SYNTHETIC-PRIVATE'},
      {'device_code': 'SYNTHETIC-PRIVATE'},
      {'verification_url': 'https://github.com.example.invalid/login/device'},
      {'verification_url': 'http://github.com/login/device'},
      {'verification_url': 'https://user@github.com/login/device'},
      {
        'verification_url':
            'https://github.com/login/device?token=SYNTHETIC-PRIVATE',
      },
      {'verification_url': 'https://github.com:444/login/device'},
      {'expires_at': '2026-01-02T10:15:00+01:00'},
      {'expires_at': '2026-02-30T10:15:00Z'},
      {'poll_interval_seconds': 0},
      {'poll_interval_seconds': 1.5},
      {'user_code': null},
    ].indexed) {
      test('device guidance rejects unsafe or invalid fields ${change.$1}', () {
        final data =
            jsonDecode(
                  jsonEncode(
                    fixture('device-session-pending')['envelope']['data'],
                  ),
                )
                as Map<String, dynamic>;
        data.addAll(change.$2);
        expect(
          () => DeviceSession.fromJson(data),
          throwsA(isA<LocalSchemaException>()),
        );
      });
    }
  });

  group('Unix HTTP client', () {
    test('status round trip uses the supplied Unix socket', () async {
      final path = await serve((request) async {
        expect(request.method, 'GET');
        expect(request.uri.path, '/v1/status');
        expect(request.headers.value('Origin'), isNull);
        expect(await request.fold<int>(0, (n, bytes) => n + bytes.length), 0);
        await reply(request, statusEnvelope());
      });
      final client = LocalClient(socketPath: path);
      addTearDown(client.close);
      final response = await client.getStatus();
      expect(response.requestId, requestId);
      expect(response.data.coverageState, 'unknown');
    });
    test(
      'rule validation sends the visible definition itself as UTF-8 JSON',
      () async {
        final definition = <String, Object?>{
          'format_version': 1,
          'condition': {'field': 'title', 'op': 'contains', 'value': 'Straße'},
        };
        final path = await serve((request) async {
          expect(request.method, 'POST');
          expect(request.uri.path, '/v1/rules/validate');
          expect(request.headers.contentType!.mimeType, 'application/json');
          expect(request.headers.contentType!.charset, 'utf-8');
          expect(
            jsonDecode(await utf8.decoder.bind(request).join()),
            definition,
          );
          await reply(
            request,
            fixture('rule-validation-summary')['envelope']
                as Map<String, dynamic>,
          );
        });
        final client = LocalClient(socketPath: path);
        addTearDown(client.close);
        expect(
          (await client.validateRule(definition)).data.summary,
          'approval count is at least 3',
        );
      },
    );
    test(
      'typed API failures retain correlation, retryability, and validation path',
      () async {
        final path = await serve((request) async {
          await reply(request, {
            'request_id': requestId,
            'error': {
              'code': 'validation_failed',
              'message': 'Invalid rule.',
              'retryable': false,
              'details': {'path': 'condition.value'},
            },
          }, status: 422);
        });
        final client = LocalClient(socketPath: path);
        addTearDown(client.close);
        await expectLater(
          client.validateRule({'format_version': 99}),
          throwsA(
            failure(LocalFailure.api)
                .having((e) => (e).requestId, 'request', requestId)
                .having((e) => (e).httpStatus, 'HTTP', 422)
                .having((e) => (e).error!.code, 'code', 'validation_failed')
                .having((e) => (e).error!.retryable, 'retryable', false)
                .having(
                  (e) => (e).error!.details['path'],
                  'path',
                  'condition.value',
                ),
          ),
        );
      },
    );
    test(
      'uninitialized host is a typed API error, not an empty list',
      () async {
        final path = await serve(
          (request) => reply(request, {
            'request_id': requestId,
            'error': {
              'code': 'host_unavailable',
              'message': 'Host is unavailable.',
              'retryable': true,
              'details': {},
            },
          }, status: 503),
        );
        final client = LocalClient(socketPath: path);
        addTearDown(client.close);
        await expectLater(
          client.getStatus(),
          throwsA(
            failure(
              LocalFailure.api,
            ).having((e) => (e).error!.code, 'code', 'host_unavailable'),
          ),
        );
      },
    );
    test('absent host is a transport failure, not empty success', () async {
      final directory = await Directory('/tmp').createTemp('sqc-');
      addTearDown(() => directory.delete(recursive: true));
      final client = LocalClient(socketPath: '${directory.path}/absent.sock');
      addTearDown(client.close);
      await expectLater(
        client.getStatus(),
        throwsA(failure(LocalFailure.hostUnavailable)),
      );
    });
    for (final protocol in ['2', 'invalid', null]) {
      test('missing or incompatible protocol header $protocol', () async {
        final path = await serve(
          (request) => reply(request, statusEnvelope(), protocol: protocol),
        );
        final client = LocalClient(socketPath: path);
        addTearDown(client.close);
        await expectLater(
          client.getStatus(),
          throwsA(failure(LocalFailure.protocolMismatch)),
        );
      });
    }
    test('correlation mismatch is not trusted', () async {
      final path = await serve(
        (request) => reply(
          request,
          statusEnvelope(),
          correlation: 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb',
        ),
      );
      final client = LocalClient(socketPath: path);
      addTearDown(client.close);
      await expectLater(
        client.getStatus(),
        throwsA(failure(LocalFailure.invalidResponse)),
      );
    });
    for (final contentType in [
      'text/html',
      'application/json; charset=iso-8859-1',
    ]) {
      test('rejects incompatible response media $contentType', () async {
        final path = await serve(
          (request) =>
              reply(request, statusEnvelope(), contentType: contentType),
        );
        final client = LocalClient(socketPath: path);
        addTearDown(client.close);
        await expectLater(
          client.getStatus(),
          throwsA(failure(LocalFailure.invalidResponse)),
        );
      });
    }
    test('success HTTP status cannot conceal an error envelope', () async {
      final path = await serve(
        (request) => reply(
          request,
          fixture('auth-required')['envelope'] as Map<String, dynamic>,
        ),
      );
      final client = LocalClient(socketPath: path);
      addTearDown(client.close);
      await expectLater(
        client.getStatus(),
        throwsA(failure(LocalFailure.invalidResponse)),
      );
    });
    test('failed HTTP status cannot be treated as successful data', () async {
      final path = await serve(
        (request) => reply(request, statusEnvelope(), status: 500),
      );
      final client = LocalClient(socketPath: path);
      addTearDown(client.close);
      await expectLater(
        client.getStatus(),
        throwsA(failure(LocalFailure.invalidResponse)),
      );
    });
    test('redirects are not followed', () async {
      var requests = 0;
      final path = await serve((request) async {
        requests++;
        request.response.statusCode = 302;
        request.response.headers.set(
          HttpHeaders.locationHeader,
          'https://example.invalid/',
        );
        await request.response.close();
      });
      final client = LocalClient(socketPath: path);
      addTearDown(client.close);
      await expectLater(
        client.getStatus(),
        throwsA(failure(LocalFailure.invalidResponse)),
      );
      expect(requests, 1);
    });
    for (final streamed in [false, true]) {
      test(
        'bounds ${streamed ? 'streamed' : 'fixed-length'} response bytes',
        () async {
          final path = await serve((request) async {
            request.response.headers.contentType = ContentType.json;
            request.response.headers.set('X-Shipq-Protocol-Version', '1');
            if (!streamed) request.response.contentLength = 1024 * 1024 + 1;
            request.response.add(List.filled(1024 * 1024 + 1, 32));
            await request.response.close();
          });
          final client = LocalClient(socketPath: path);
          addTearDown(client.close);
          await expectLater(
            client.getStatus(),
            throwsA(failure(LocalFailure.invalidResponse)),
          );
        },
      );
    }
    test('accepts a valid response at the exact byte limit', () async {
      final body = utf8.encode(jsonEncode(statusEnvelope()));
      final path = await serve((request) async {
        request.response.headers.contentType = ContentType.json;
        request.response.headers.set('X-Shipq-Protocol-Version', '1');
        request.response.headers.set('X-Request-ID', requestId);
        request.response.contentLength = 1024 * 1024;
        request.response.add(body);
        request.response.add(List.filled(1024 * 1024 - body.length, 32));
        await request.response.close();
      });
      final client = LocalClient(socketPath: path);
      addTearDown(client.close);
      expect((await client.getStatus()).data.coverageState, 'unknown');
    });
    for (final bytes in [
      [0xff],
      utf8.encode('{"request_id":"$requestId","data":"SYNTHETIC-PRIVATE"'),
    ].indexed) {
      test('invalid UTF-8/JSON fails safely ${bytes.$1}', () async {
        final path = await serve((request) async {
          request.response.headers.contentType = ContentType.json;
          request.response.headers.set('X-Shipq-Protocol-Version', '1');
          request.response.add(bytes.$2);
          await request.response.close();
        });
        final client = LocalClient(socketPath: path);
        addTearDown(client.close);
        await expectLater(
          client.getStatus(),
          throwsA(
            failure(LocalFailure.invalidResponse).having(
              (error) => error.toString(),
              'safe message',
              isNot(contains('SYNTHETIC-PRIVATE')),
            ),
          ),
        );
      });
    }
    test(
      'timeout releases its connection without cancelling another request',
      () async {
        final received = Completer<void>();
        final release = Completer<void>();
        var requests = 0;
        final path = await serve((request) async {
          if (++requests == 1) {
            received.complete();
            await release.future;
          } else {
            await reply(request, statusEnvelope());
          }
        });
        addTearDown(release.complete);
        final client = LocalClient(
          socketPath: path,
          timeout: const Duration(seconds: 1),
        );
        addTearDown(client.close);
        final first = expectLater(
          client.getStatus(),
          throwsA(failure(LocalFailure.timeout)),
        );
        await received.future;
        expect((await client.getStatus()).data.monitorState, 'unavailable');
        await first;
        expect((await client.getStatus()).data.coverageState, 'unknown');
      },
    );
    test('the deadline also bounds a stalled streamed response', () async {
      final started = Completer<void>();
      final release = Completer<void>();
      final path = await serve((request) async {
        request.response.headers.contentType = ContentType.json;
        request.response.headers.set('X-Shipq-Protocol-Version', '1');
        request.response.add(utf8.encode('{"request_id":"$requestId",'));
        await request.response.flush();
        started.complete();
        await release.future;
      });
      addTearDown(release.complete);
      final client = LocalClient(
        socketPath: path,
        timeout: const Duration(seconds: 1),
      );
      addTearDown(client.close);
      final pending = expectLater(
        client.getStatus(),
        throwsA(failure(LocalFailure.timeout)),
      );
      await started.future;
      await pending;
    });
    test('close cancels in-flight work and prevents reconnection', () async {
      final received = Completer<void>();
      final release = Completer<void>();
      final path = await serve((request) async {
        received.complete();
        await release.future;
      });
      addTearDown(release.complete);
      final client = LocalClient(socketPath: path);
      addTearDown(client.close);
      final pending = expectLater(
        client.getStatus(),
        throwsA(failure(LocalFailure.closed)),
      );
      await received.future;
      client.close();
      await pending;
      await expectLater(
        client.getStatus(),
        throwsA(failure(LocalFailure.closed)),
      );
      client.close();
    });
    test('client exceptions never stringify remote messages or details', () {
      final error = LocalApiError.fromJson({
        'code': 'internal',
        'message': 'SYNTHETIC-PRIVATE',
        'retryable': false,
        'details': {'message': 'SYNTHETIC-PRIVATE'},
      });
      expect(
        LocalClientException(LocalFailure.api, error: error).toString(),
        isNot(contains('SYNTHETIC-PRIVATE')),
      );
    });
    test('socket paths and deadlines must be valid before any connection', () {
      for (final path in [
        '',
        'relative.sock',
        '/tmp/api\u0000.sock',
        '/${'ü' * 52}',
      ]) {
        expect(() => LocalClient(socketPath: path), throwsArgumentError);
      }
      expect(
        () => LocalClient(socketPath: '/tmp/api.sock', timeout: Duration.zero),
        throwsArgumentError,
      );
    });
  });
}
