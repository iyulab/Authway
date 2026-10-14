"""Probe: can the authorization server issue a delegated token behind a token
endpoint that the API fronts?

Runs against a disposable Ory Hydra (v26.2.0) started by run.ps1 with an
in-memory store, and answers four questions the delegation design rests on:

  1. With webfinger.oidc_discovery.token_url pointing at a front endpoint,
     does discovery advertise it, and does the engine accept a client
     assertion whose `aud` is that front URL (so the front can forward the
     client's own assertion unchanged)? And one whose `aud` is the engine's
     own token URL? And does it refuse any other `aud`?
  2. Does the jwt-bearer grant (RFC 7523) issue a token whose `sub` is the
     assertion's subject and whose `client_id` is the requesting client, with
     the client's own jwt-bearer lifespan?
  3. Does the token hook reach the hook endpoint with the configured API-key
     header, put `act` at the top level for the jwt-bearer grant, and leave a
     client_credentials token alone when it answers 204?
  4. Does a 403 from the hook refuse the grant?

Usage: python -I probe.py   (after run.ps1 has started the engine)
Prints one line per check and exits non-zero if any check fails.
"""
import base64
import http.server
import json
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid

from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import padding, rsa

PUBLIC = 'http://localhost:4544'
ADMIN = 'http://localhost:4545'
ENGINE_TOKEN = PUBLIC + '/oauth2/token'
FRONT_TOKEN = 'http://localhost:9999/oauth2/token'  # stands in for the API's front
HOOK_PORT = 9911
HOOK_KEY = 'probe-hook-key'

failures = []


def check(name, ok, detail=''):
    print(('PASS ' if ok else 'FAIL ') + name + (f' — {detail}' if detail else ''))
    if not ok:
        failures.append(name)


def b64u(b):
    return base64.urlsafe_b64encode(b).rstrip(b'=').decode()


def b64u_int(i):
    return b64u(i.to_bytes((i.bit_length() + 7) // 8, 'big'))


class Key:
    def __init__(self, kid):
        self.kid = kid
        self.priv = rsa.generate_private_key(public_exponent=65537, key_size=2048)
        pub = self.priv.public_key().public_numbers()
        self.jwk = {'kty': 'RSA', 'kid': kid, 'alg': 'RS256', 'use': 'sig', 'n': b64u_int(pub.n), 'e': b64u_int(pub.e)}

    def sign(self, claims):
        header = {'alg': 'RS256', 'typ': 'JWT', 'kid': self.kid}
        signing_input = (b64u(json.dumps(header).encode()) + '.' + b64u(json.dumps(claims).encode())).encode()
        sig = self.priv.sign(signing_input, padding.PKCS1v15(), hashes.SHA256())
        return signing_input.decode() + '.' + b64u(sig)


def call(method, url, body=None, form=None, headers=None):
    data = None
    h = dict(headers or {})
    if form is not None:
        data = urllib.parse.urlencode(form).encode()
        h['Content-Type'] = 'application/x-www-form-urlencoded'
    elif body is not None:
        data = json.dumps(body).encode()
        h['Content-Type'] = 'application/json'
    req = urllib.request.Request(url, data=data, method=method, headers=h)
    try:
        with urllib.request.urlopen(req, timeout=15) as r:
            raw = r.read()
            return r.status, (json.loads(raw) if raw else None)
    except urllib.error.HTTPError as e:
        raw = e.read()
        try:
            return e.code, json.loads(raw)
        except Exception:
            return e.code, raw.decode(errors='replace')


def claims_of(jwt):
    p = jwt.split('.')[1]
    return json.loads(base64.urlsafe_b64decode(p + '=' * (-len(p) % 4)))


# ---- hook receiver -------------------------------------------------------
hook_calls = []
hook_mode = {'deny': False}


class Hook(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers.get('Content-Length', 0))) or b'{}')
        grant = (body.get('request') or {}).get('grant_types') or []
        hook_calls.append({'key': self.headers.get('X-Hook-Key'), 'grant': grant, 'body': body})
        if hook_mode['deny']:
            self.send_response(403)
            self.end_headers()
            return
        if 'urn:ietf:params:oauth:grant-type:jwt-bearer' in grant:
            out = {'session': {'access_token': {'act': {'sub': body['request'].get('client_id')}}}}
            data = json.dumps(out).encode()
            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.send_header('Content-Length', str(len(data)))
            self.end_headers()
            self.wfile.write(data)
            return
        self.send_response(204)
        self.end_headers()

    def log_message(self, *a):
        pass


server = http.server.HTTPServer(('0.0.0.0', HOOK_PORT), Hook)
threading.Thread(target=server.serve_forever, daemon=True).start()

# ---- 1. discovery and the accepted audiences ------------------------------
for _ in range(30):
    try:
        st, disco = call('GET', PUBLIC + '/.well-known/openid-configuration')
        if st == 200:
            break
    except Exception:
        pass
    time.sleep(1)
check('discovery advertises the front token endpoint', disco.get('token_endpoint') == FRONT_TOKEN, disco.get('token_endpoint'))

client_key = Key('actor-key-1')
issuer_key = Key('delegation-key-1')
client_id = 'actor-' + uuid.uuid4().hex[:8]
st, client = call('POST', ADMIN + '/admin/clients', body={
    'client_id': client_id,
    'grant_types': ['client_credentials', 'urn:ietf:params:oauth:grant-type:jwt-bearer'],
    'token_endpoint_auth_method': 'private_key_jwt',
    'token_endpoint_auth_signing_alg': 'RS256',
    'jwks': {'keys': [client_key.jwk]},
    'scope': 'orders.read orders.write',
    'audience': ['https://orders.example'],
    'access_token_strategy': 'jwt',
    'jwt_bearer_grant_access_token_lifespan': '5m0s',
})
check('register a key client with the jwt-bearer grant', st == 201, str(st))

st, _ = call('POST', ADMIN + '/admin/trust/grants/jwt-bearer/issuers', body={
    'issuer': 'https://api.example/delegation',
    'allow_any_subject': True,
    'scope': ['orders.read'],
    'jwk': issuer_key.jwk,
    'expires_at': '2099-01-01T00:00:00Z',
})
check('register the API as a trusted issuer', st == 201, str(st))


def client_assertion(aud):
    now = int(time.time())
    return client_key.sign({'iss': client_id, 'sub': client_id, 'aud': aud, 'jti': uuid.uuid4().hex, 'iat': now, 'exp': now + 120})


def subject_assertion(sub='user-1', aud=FRONT_TOKEN):
    now = int(time.time())
    return issuer_key.sign({'iss': 'https://api.example/delegation', 'sub': sub, 'aud': aud, 'jti': uuid.uuid4().hex, 'iat': now, 'exp': now + 60})


def delegated(client_aud, subject_aud=FRONT_TOKEN, scope='orders.read'):
    return call('POST', ENGINE_TOKEN, form={
        'grant_type': 'urn:ietf:params:oauth:grant-type:jwt-bearer',
        'assertion': subject_assertion(aud=subject_aud),
        'scope': scope,
        'audience': 'https://orders.example',
        'client_assertion_type': 'urn:ietf:params:oauth:client-assertion-type:jwt-bearer',
        'client_assertion': client_assertion(client_aud),
    })


st, tok = delegated(FRONT_TOKEN)
check('client assertion addressed to the front token URL is accepted', st == 200, f'{st} {tok if st != 200 else ""}')
st2, _ = delegated(ENGINE_TOKEN, subject_aud=ENGINE_TOKEN)
check("client assertion addressed to the engine's own token URL is accepted", st2 == 200, str(st2))
st3, body3 = delegated('https://elsewhere.example/token')
check('client assertion addressed anywhere else is refused', st3 == 401, f'{st3} {body3}')

# ---- 2. the delegated token ------------------------------------------------
if st == 200:
    c = claims_of(tok['access_token'])
    check('token subject is the delegating user', c.get('sub') == 'user-1', c.get('sub'))
    check('token client_id is the actor', c.get('client_id') == client_id, c.get('client_id'))
    # The engine counts the lifespan from when the request arrived and stamps
    # iat when it signs, after the hook answered, so exp - iat comes out a few
    # seconds under 300. What matters is that it is the client's 5 minutes,
    # not the engine default of an hour.
    life = c.get('exp', 0) - c.get('iat', 0)
    check('token lifetime is the client jwt-bearer lifespan (5 min, not the 1 h default)', 280 <= life <= 300, str(life))
    check('no refresh token, no id token', 'refresh_token' not in tok and 'id_token' not in tok)
    check('aud contains the requested audience', 'https://orders.example' in (c.get('aud') or []), str(c.get('aud')))
    print('INFO aud as issued: ' + json.dumps(c.get('aud')))
    # ---- 3. the hook ----------------------------------------------------------
    check('act is a top-level claim naming the actor', (c.get('act') or {}).get('sub') == client_id, json.dumps(c.get('act')))

jb_calls = [h for h in hook_calls if 'urn:ietf:params:oauth:grant-type:jwt-bearer' in h['grant']]
check('hook was called for the jwt-bearer grant', len(jb_calls) >= 1, str(len(jb_calls)))
check('hook received the API-key header', all(h['key'] == HOOK_KEY for h in hook_calls) and hook_calls, str({h['key'] for h in hook_calls}))
if jb_calls:
    req = jb_calls[0]['body'].get('request') or {}
    check('hook receives the subject assertion as sent', 'assertion' in (req.get('payload') or {}), json.dumps(sorted((req.get('payload') or {}).keys())))

before = len(hook_calls)
st, cc = call('POST', ENGINE_TOKEN, form={
    'grant_type': 'client_credentials',
    'scope': 'orders.read',
    'client_assertion_type': 'urn:ietf:params:oauth:client-assertion-type:jwt-bearer',
    'client_assertion': client_assertion(FRONT_TOKEN),
})
check('client_credentials still works with the hook answering 204', st == 200, str(st))
check('the hook is called for client_credentials too', len(hook_calls) > before)
if st == 200:
    check('a 204 from the hook adds nothing', 'act' not in claims_of(cc['access_token']))

# ---- 4. a refusing hook -----------------------------------------------------
hook_mode['deny'] = True
st, body = delegated(FRONT_TOKEN)
check('a 403 from the hook refuses the grant', st == 403, f'{st} {body}')
hook_mode['deny'] = False

server.shutdown()
print(f'\n{len(failures)} failed' if failures else '\nall checks passed')
raise SystemExit(1 if failures else 0)
