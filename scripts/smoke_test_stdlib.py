#!/usr/bin/env python3
"""
Smoke test using only Python stdlib (urllib) so no external deps needed.
"""
import json
import time
import os
import sys
from urllib import request, error, parse
import base64

BASE = "http://127.0.0.1:8080"

def do_request(method, path, data=None, headers=None, timeout=5):
    url = BASE + path
    b = None
    if data is not None:
        b = json.dumps(data).encode()
    req = request.Request(url, data=b, method=method)
    req.add_header('Content-Type', 'application/json')
    if headers:
        for k, v in headers.items():
            req.add_header(k, v)
    try:
        with request.urlopen(req, timeout=timeout) as resp:
            body = resp.read().decode()
            code = resp.getcode()
            return code, body
    except error.HTTPError as e:
        return e.code, e.read().decode()
    except Exception as e:
        print('Request error', e)
        return None, str(e)

print('1) Registering user')
email = f'smoke2+{int(time.time())}@example.com'
password = 'password123'
code, body = do_request('POST', '/auth/register', {'email': email, 'password': password})
print(code, body)
if code == 400 and 'duplicate' in body.lower():
    # Already exists — try to login
    print('user exists, logging in')
    code, body = do_request('POST', '/auth/login', {'email': email, 'password': password})
    print('login:', code, body)
    if code != 200:
        print('login failed')
        sys.exit(1)
elif code != 200:
    print('register failed')
    sys.exit(1)

try:
    tok = json.loads(body).get('token')
except Exception:
    print('invalid auth response')
    sys.exit(1)

print('token:', tok)
headers = {'Authorization': f'Bearer {tok}'}

# extract user_id from JWT payload if possible
user_id = None
try:
    parts = tok.split('.')
    if len(parts) >= 2:
        padded = parts[1] + '=' * (-len(parts[1]) % 4)
        payload = json.loads(base64.urlsafe_b64decode(padded).decode())
        user_id = int(payload.get('user_id', 0))
except Exception:
    user_id = None
print('user_id from token:', user_id)

print('2) Creating account')
code, body = do_request('POST', '/v1/accounts', {'user_id': user_id or 1, 'name': 'Smoke Acc', 'currency': 'USD'}, headers=headers)
print(code, body)
if code not in (200,201):
    print('create account failed')
    sys.exit(1)
acc = json.loads(body)
print('account id', acc.get('id'))

print('3) Creating transaction')
now = time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime())
code, body = do_request('POST', '/v1/transactions', {
    'user_id': 1,
    'account_id': acc.get('id'),
    'amount_minor': 500,
    'currency': 'USD',
    'occurred_at': now,
    'note': 'smoke stdlib'
}, headers=headers)
print(code, body)
if code != 200:
    print('create tx failed')
    sys.exit(1)

print('4) Requesting export')
month = time.strftime('%Y-%m', time.gmtime())
code, body = do_request('POST', f'/exports?month={month}', None, headers=headers)
print(code, body)
if code != 200:
    print('create export failed')
    sys.exit(1)
exp = json.loads(body)
# handle different JSON key styles (sqlc may emit PascalCase)
exp_id = exp.get('id') or exp.get('ID') or exp.get('Id')
file_path_reported = exp.get('file_path') or exp.get('FilePath') or exp.get('filePath')
print('exp raw:', exp)
print('exp id', exp_id)

print('5) Polling for status')
status = None
data = None
for i in range(30):
    code, body = do_request('GET', f'/exports/{exp_id}/status')
    print('poll', i, code, body)
    if code != 200:
        time.sleep(1)
        continue
    data = json.loads(body)
    # status field variants
    status = data.get('status') or data.get('Status')
    if status == 'done':
        break
    if status == 'error':
        print('export errored')
        sys.exit(1)
    time.sleep(1)

if status != 'done':
    print('export not finished in time')
    sys.exit(1)

file_path = data.get('file_path') or data.get('FilePath') or file_path_reported
print('file_path reported:', file_path)
if not file_path:
    print('no file_path')
    sys.exit(1)

if os.path.exists(file_path):
    print('file exists:', file_path)
    print('--- file content (first 400 bytes) ---')
    with open(file_path, 'rb') as f:
        print(f.read(400))
else:
    print('file not found on disk at reported path')
    # try local tmp/exports
    local = os.path.join(os.path.dirname(__file__), 'exports', os.path.basename(file_path))
    if os.path.exists(local):
        print('found in tmp/exports:', local)
        with open(local, 'rb') as f:
            print(f.read(400))
    else:
        print('not found locally either')
        sys.exit(1)

print('SMOKE OK')
