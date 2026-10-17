#!/usr/bin/env python3
"""Synthetic OpenAI-compatible acceptance probe. Never prints API responses."""
import argparse
import json
import os
from pathlib import Path
import urllib.parse
import urllib.request

from managed_update import NoRedirects, UpdateError, read_json, require, secure_file


def config(path, uid=0, anchor=Path('/')):
    value = read_json(path, uid, anchor, private=True)
    required = {'schema_version', 'base_url', 'key_file', 'chat_model', 'vision_model'}
    optional = {'embedding_model', 'embedding_dimensions', 'max_tokens', 'timeout', 'request_fields'}
    require(required <= set(value) <= required | optional and value['schema_version'] == 1, 'PROBE_CONFIG_INVALID')
    url = urllib.parse.urlsplit(value['base_url'])
    require(url.scheme == 'https' or (url.scheme == 'http' and url.hostname in ('127.0.0.1', '::1', 'localhost')), 'PROBE_TLS_REQUIRED')
    require(url.hostname and url.username is None and url.password is None and not url.query and not url.fragment and value['base_url'].endswith('/v1'), 'PROBE_URL_INVALID')
    for field in ('chat_model', 'vision_model'):
        require(isinstance(value[field], str) and 0 < len(value[field]) <= 200, 'PROBE_MODEL_REQUIRED')
    if 'embedding_model' in value:
        require(isinstance(value['embedding_model'], str) and 0 < len(value['embedding_model']) <= 200 and type(value.get('embedding_dimensions')) is int and 1 <= value['embedding_dimensions'] <= 65536, 'PROBE_EMBEDDING_INVALID')
    for field, default, maximum in [('max_tokens', 2048, 4096), ('timeout', 30, 60)]:
        value.setdefault(field, default)
        require(type(value[field]) is int and 1 <= value[field] <= maximum, 'PROBE_BUDGET_INVALID')
    fields = value.get('request_fields', {})
    require(isinstance(fields, dict) and set(fields) <= {'store', 'reasoning_effort', 'prompt_cache_key', 'prompt_cache_retention'}, 'PROBE_REQUEST_FIELDS_INVALID')
    require(len(json.dumps(fields)) <= 1024, 'PROBE_REQUEST_FIELDS_INVALID')
    secure_file(Path(value['key_file']), uid, anchor, private=True)
    return value


class Probe:
    def __init__(self, settings):
        self.settings = settings
        key = Path(settings['key_file']).read_text().strip()
        require(0 < len(key) <= 4096 and not any(c in key for c in '\r\n'), 'PROBE_KEY_INVALID')
        self.key = key
        self.opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirects)

    def request(self, endpoint, payload, stream=False):
        data = json.dumps(payload).encode()
        request = urllib.request.Request(self.settings['base_url'] + endpoint, data=data, headers={'Authorization': 'Bearer ' + self.key, 'Content-Type': 'application/json'})
        try:
            with self.opener.open(request, timeout=self.settings['timeout']) as response:
                raw = response.read(1048577)
            require(len(raw) <= 1048576, 'PROBE_RESPONSE_TOO_LARGE')
            return raw.decode() if stream else json.loads(raw)
        except UpdateError:
            raise
        except Exception:
            raise UpdateError('PROBE_REQUEST_FAILED') from None

    def chat(self, messages, **extra):
        payload = dict(model=self.settings['chat_model'], messages=messages, max_tokens=self.settings['max_tokens'], **self.settings.get('request_fields', {}))
        payload.update(extra)
        return self.request('/chat/completions', payload, stream=bool(payload.get('stream')))

    @staticmethod
    def message(response):
        try:
            result = response['choices'][0]['message']
            require(isinstance(result, dict), 'PROBE_MESSAGE_INVALID')
            return result
        except (KeyError, TypeError, IndexError):
            raise UpdateError('PROBE_MESSAGE_INVALID') from None

    def run(self):
        user = {'role': 'user', 'content': 'Reply exactly AIL_OK. This is a synthetic deployment test.'}
        answer = self.message(self.chat([user]))
        require('AIL_OK' in (answer.get('content') or ''), 'PROBE_TEXT_FAILED')
        tools = [{'type': 'function', 'function': {'name': 'ail_echo', 'description': 'Synthetic echo test only', 'parameters': {'type': 'object', 'properties': {'text': {'type': 'string'}}, 'required': ['text']}}}]
        prompt = {'role': 'user', 'content': 'Call ail_echo with text AIL_TOOL_OK, then repeat its result.'}
        assistant = self.message(self.chat([prompt], tools=tools, tool_choice={'type': 'function', 'function': {'name': 'ail_echo'}}))
        calls = assistant.get('tool_calls')
        require(isinstance(calls, list) and len(calls) == 1, 'PROBE_TOOL_FAILED')
        call = calls[0]
        require(call.get('id') and call.get('function', {}).get('name') == 'ail_echo', 'PROBE_TOOL_FAILED')
        require(json.loads(call['function']['arguments']).get('text') == 'AIL_TOOL_OK', 'PROBE_TOOL_ARGUMENTS_FAILED')
        result = {'role': 'tool', 'tool_call_id': call['id'], 'content': 'AIL_TOOL_OK'}
        replay = self.message(self.chat([prompt, assistant, result], tools=tools))
        require('AIL_TOOL_OK' in (replay.get('content') or ''), 'PROBE_REPLAY_FAILED')
        # Exercise clients that discard vendor-specific assistant fields.
        normalized = {k: assistant[k] for k in ('role', 'content', 'tool_calls') if k in assistant}
        replay = self.message(self.chat([prompt, normalized, result], tools=tools))
        require('AIL_TOOL_OK' in (replay.get('content') or ''), 'PROBE_NORMALIZED_REPLAY_FAILED')
        # 1x1 opaque red PNG, synthetic and embedded. No private file/image.
        png = 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4z8DwHwAFAAH/iZk9HQAAAABJRU5ErkJggg=='
        vision = self.message(self.chat([{'role': 'user', 'content': [{'type': 'text', 'text': 'Name the color of this image. Reply with red.'}, {'type': 'image_url', 'image_url': {'url': 'data:image/png;base64,' + png}}]}], model=self.settings['vision_model']))
        require('red' in (vision.get('content') or '').lower(), 'PROBE_VISION_FAILED')
        events = self.chat([user], stream=True)
        text = ''
        done = False
        for line in events.splitlines():
            if not line.startswith('data:'):
                continue
            body = line[5:].strip()
            if body == '[DONE]':
                done = True
                continue
            frame = json.loads(body)
            for choice in frame.get('choices', []):
                text += choice.get('delta', {}).get('content') or ''
        require(done and 'AIL_OK' in text, 'PROBE_STREAM_FAILED')
        if self.settings.get('embedding_model'):
            response = self.request('/embeddings', {'model': self.settings['embedding_model'], 'input': 'Synthetic local embedding acceptance probe.'})
            require(len(response['data'][0]['embedding']) == self.settings['embedding_dimensions'], 'PROBE_EMBEDDING_FAILED')
        return {'ok': True, 'checks': ['text', 'tool', 'replay', 'normalized_replay', 'vision', 'stream'] + (['embedding'] if self.settings.get('embedding_model') else [])}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--config', required=True, type=Path)
    parser.add_argument('--validate-only', action='store_true')
    args = parser.parse_args()
    try:
        require(os.geteuid() == 0, 'ROOT_REQUIRED')
        settings = config(args.config)
        result = {'valid': True} if args.validate_only else Probe(settings).run()
        print(json.dumps(result))
        return 0
    except UpdateError as error:
        print(json.dumps({'ok': False, 'code': str(error)}))
        return 1
    except Exception:
        print(json.dumps({'ok': False, 'code': 'PROBE_FAILED'}))
        return 1


if __name__ == '__main__':
    raise SystemExit(main())
