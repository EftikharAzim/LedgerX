#!/usr/bin/env python3
"""
Smoke test using only Python stdlib (urllib) so no external deps needed.

Covers the double-entry API end to end: register, accounts, idempotent
transaction creation (same key twice -> same transaction), transfer,
balances, entries listing, summary, and the authenticated export flow.
"""
import json
import time
import os
import sys
import uuid
from urllib import request, error

BASE = os.environ.get("LEDGERX_BASE_URL", "http://127.0.0.1:8080")


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
            return resp.getcode(), resp.read().decode()
    except error.HTTPError as e:
        return e.code, e.read().decode()
    except Exception as e:
        print('Request error', e)
        return None, str(e)


def fail(msg, code=None, body=None):
    print(f'FAIL: {msg}', code or '', body or '')
    sys.exit(1)


print('1) Registering user')
email = f'smoke2+{int(time.time())}@example.com'
password = 'password123'
code, body = do_request('POST', '/auth/register', {'email': email, 'password': password})
print(code, body)
if code not in (200, 201):
    fail('register failed', code, body)
tok = json.loads(body).get('token')
if not tok:
    fail('no token in auth response')
headers = {'Authorization': f'Bearer {tok}'}

print('2) Creating two accounts')
code, body = do_request('POST', '/v1/accounts', {'name': 'Smoke Checking', 'currency': 'USD'}, headers=headers)
print(code, body)
if code not in (200, 201):
    fail('create account failed', code, body)
acc1 = json.loads(body)
code, body = do_request('POST', '/v1/accounts', {'name': 'Smoke Savings', 'currency': 'USD'}, headers=headers)
if code not in (200, 201):
    fail('create second account failed', code, body)
acc2 = json.loads(body)
print('accounts', acc1.get('id'), acc2.get('id'))

print('3) Creating transaction with idempotency key (sent twice)')
now = time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime())
idem = str(uuid.uuid4())
tx_body = {
    'account_id': acc1.get('id'),
    'amount_minor': 500,
    'currency': 'USD',
    'occurred_at': now,
    'note': 'smoke stdlib',
}
idem_headers = dict(headers, **{'Idempotency-Key': idem})
code, body = do_request('POST', '/v1/transactions', tx_body, headers=idem_headers)
print(code, body)
if code not in (200, 201):
    fail('create tx failed', code, body)
tx1 = json.loads(body)
postings = tx1.get('postings') or []
if sum(p['amount_minor'] for p in postings) != 0:
    fail('postings do not sum to zero', body=body)

code, body = do_request('POST', '/v1/transactions', tx_body, headers=idem_headers)
print('replay:', code, body)
if code not in (200, 201):
    fail('idempotent replay failed', code, body)
tx2 = json.loads(body)
if tx1.get('id') != tx2.get('id'):
    fail(f"idempotent replay created a new transaction: {tx1.get('id')} vs {tx2.get('id')}")

print('4) Transfer between accounts')
code, body = do_request('POST', '/v1/transfers', {
    'from_account_id': acc1.get('id'),
    'to_account_id': acc2.get('id'),
    'amount_minor': 200,
    'currency': 'USD',
    'occurred_at': now,
}, headers=headers)
print(code, body)
if code not in (200, 201):
    fail('transfer failed', code, body)

print('5) Balances reflect tx + transfer')
code, body = do_request('GET', f"/v1/accounts/{acc1.get('id')}/balance", headers=headers)
print(code, body)
if code != 200 or json.loads(body).get('balance_minor') != 300:
    fail('account 1 balance should be 300', code, body)
code, body = do_request('GET', f"/v1/accounts/{acc2.get('id')}/balance", headers=headers)
print(code, body)
if code != 200 or json.loads(body).get('balance_minor') != 200:
    fail('account 2 balance should be 200', code, body)

print('6) Entries listing')
code, body = do_request('GET', f"/v1/accounts/{acc1.get('id')}/transactions", headers=headers)
print(code, body)
if code != 200 or len(json.loads(body).get('entries', [])) != 2:
    fail('expected 2 entries on account 1', code, body)

print('7) Monthly summary')
month = time.strftime('%Y-%m', time.gmtime())
code, body = do_request('GET', f"/v1/accounts/{acc1.get('id')}/summary?month={month}", headers=headers)
print(code, body)
if code != 200:
    fail('summary failed', code, body)

print('7b) Reversal')
code, body = do_request('POST', f"/v1/transactions/{tx1.get('id')}/reverse", None, headers=headers)
print(code, body)
if code not in (200, 201):
    fail('reversal failed', code, body)
rev = json.loads(body)
if rev.get('reversal_of') != tx1.get('id'):
    fail('reversal_of mismatch', body=body)
code, body = do_request('POST', f"/v1/transactions/{tx1.get('id')}/reverse", None, headers=headers)
if code != 409:
    fail(f'double reversal should be 409, got {code}', body=body)
code, body = do_request('GET', f"/v1/accounts/{acc1.get('id')}/balance", headers=headers)
if code != 200 or json.loads(body).get('balance_minor') != -200:
    fail('account 1 balance after reversal should be -200 (300 - 500)', code, body)

print('8) Requesting export')
code, body = do_request('POST', f'/v1/exports?month={month}', None, headers=headers)
print(code, body)
if code not in (200, 202):
    fail('create export failed', code, body)
exp_id = json.loads(body).get('id')
print('exp id', exp_id)

print('8a) Export status requires auth')
code, _ = do_request('GET', f'/v1/exports/{exp_id}/status')
if code != 401:
    fail(f'unauthenticated export status should be 401, got {code}')

print('9) Polling for status')
status = None
data = None
for i in range(30):
    code, body = do_request('GET', f'/v1/exports/{exp_id}/status', headers=headers)
    print('poll', i, code, body)
    if code != 200:
        time.sleep(1)
        continue
    data = json.loads(body)
    status = data.get('status') or data.get('Status')
    if status == 'done':
        break
    if status == 'error':
        fail('export errored')
    time.sleep(1)

if status != 'done':
    fail('export not finished in time')

print('10) Downloading export via API')
code, body = do_request('GET', f'/v1/exports/{exp_id}/download', headers=headers)
if code != 200:
    fail('download failed', code, body)
if not body.startswith('transaction_id,account_id,account_name'):
    fail('unexpected CSV header', body=body[:200])
lines = [l for l in body.strip().split('\n') if l]
if len(lines) < 4:  # header + tx + 2 transfer legs (+ reversal legs)
    fail(f'expected at least 4 CSV lines, got {len(lines)}', body=body[:400])
print('--- CSV (first 400 bytes) ---')
print(body[:400])

print('SMOKE OK')
