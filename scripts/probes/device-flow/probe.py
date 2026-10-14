"""Probe: the authorization server's device authorization grant (RFC 8628),
end to end, with this script playing the device, the browser and the login
UI's backend.

Runs against a disposable Ory Hydra (v26.2.0) started by run.ps1 with an
in-memory store. Answers what the device sign-in design rests on:

  1. What the device authorization response carries, and where
     verification_uri points.
  2. How the browser reaches the login UI's user-code page (the query it
     carries — device_challenge, and the user code when the complete URI is
     used).
  3. What accepting the user code returns, and that a wrong code is refused.
  4. That the flow then runs through the same login and consent requests as
     an authorization-code sign-in, and ends on the configured success page.
  5. What the device receives when it polls: authorization_pending before,
     tokens after (which ones), and that the device code works once.

Usage: python -I probe.py   (after run.ps1 has started the engine)
"""
import http.cookiejar
import json
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid

PUBLIC = 'http://localhost:4644'
ADMIN = 'http://localhost:4645'
UI = 'http://localhost:9914'

failures = []


def check(name, ok, detail=''):
    print(('PASS ' if ok else 'FAIL ') + name + (f' — {detail}' if detail else ''))
    if not ok:
        failures.append(name)


def call(method, url, body=None, form=None):
    data, h = None, {}
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


# A browser that keeps cookies and stops at the login UI (which does not run
# here): it follows the engine's redirects and reports where it lands.
jar = http.cookiejar.CookieJar()


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *a, **k):
        return None


browser_opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar), NoRedirect)


def browse(url, hops=10):
    for _ in range(hops):
        if url.startswith(UI):
            return url
        try:
            r = browser_opener.open(url, timeout=15)
            return f'{r.status} {url}'
        except urllib.error.HTTPError as e:
            if e.code in (301, 302, 303, 307, 308):
                url = urllib.parse.urljoin(url, e.headers['Location'])
                continue
            return f'{e.code} {url} {e.read()[:200]!r}'
    return url


def query(url):
    return dict(urllib.parse.parse_qsl(urllib.parse.urlparse(url).query))


for _ in range(30):
    try:
        st, disco = call('GET', PUBLIC + '/.well-known/openid-configuration')
        if st == 200:
            break
    except Exception:
        pass
    time.sleep(1)
check('discovery advertises the device authorization endpoint', disco.get('device_authorization_endpoint') == PUBLIC + '/oauth2/device/auth', disco.get('device_authorization_endpoint'))
check('discovery lists the device_code grant', 'urn:ietf:params:oauth:grant-type:device_code' in disco.get('grant_types_supported', []))

client_id = 'tv-' + uuid.uuid4().hex[:8]
st, _ = call('POST', ADMIN + '/admin/clients', body={
    'client_id': client_id,
    'grant_types': ['urn:ietf:params:oauth:grant-type:device_code', 'refresh_token'],
    'token_endpoint_auth_method': 'none',
    'scope': 'openid offline_access profile',
})
check('register a public client with the device_code grant', st == 201, str(st))

# ---- 1. device authorization -------------------------------------------------
st, dev = call('POST', PUBLIC + '/oauth2/device/auth', form={'client_id': client_id, 'scope': 'openid offline_access profile'})
check('device authorization answers 200', st == 200, f'{st} {dev if st != 200 else ""}')
print('INFO device authorization response keys: ' + json.dumps(sorted(dev.keys())) if isinstance(dev, dict) else '')
print('INFO verification_uri: ' + str(dev.get('verification_uri')))
print('INFO verification_uri_complete: ' + str(dev.get('verification_uri_complete')))
print(f"INFO user_code: length {len(dev.get('user_code', ''))}, sample shape {''.join('A' if c.isalpha() else '9' if c.isdigit() else c for c in dev.get('user_code', ''))}")
print(f"INFO expires_in {dev.get('expires_in')}, interval {dev.get('interval')}")
check('verification_uri is the engine device/verify page', str(dev.get('verification_uri', '')).startswith(PUBLIC + '/oauth2/device/verify'))


def poll():
    return call('POST', PUBLIC + '/oauth2/token', form={
        'grant_type': 'urn:ietf:params:oauth:grant-type:device_code',
        'device_code': dev['device_code'],
        'client_id': client_id,
    })


st, body = poll()
check('polling before approval: authorization_pending', st == 400 and isinstance(body, dict) and body.get('error') == 'authorization_pending', f'{st} {body}')

# ---- 2. the browser reaches the user-code page --------------------------------
landed = browse(dev['verification_uri'])
check('verification_uri sends the browser to the login UI device page', landed.startswith(UI + '/device'), landed)
q = query(landed)
print('INFO device page query keys: ' + json.dumps(sorted(q.keys())))
challenge = q.get('device_challenge')
check('the device page receives a device_challenge', bool(challenge))

# The complete URI is opened in a second browser: in the same one, the
# visit would replace the first visit's CSRF cookie, and only the device page
# opened last in a browser can finish.
other_browser = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()), NoRedirect)


def browse_with(opener, url, hops=10):
    for _ in range(hops):
        if url.startswith(UI):
            return url
        try:
            r = opener.open(url, timeout=15)
            return f'{r.status} {url}'
        except urllib.error.HTTPError as e:
            if e.code in (301, 302, 303, 307, 308):
                url = urllib.parse.urljoin(url, e.headers['Location'])
                continue
            return f'{e.code} {url}'
    return url


landed_complete = browse_with(other_browser, dev['verification_uri_complete']) if dev.get('verification_uri_complete') else ''
qc = query(landed_complete) if landed_complete.startswith(UI) else {}
check('the complete URI brings the user code to the device page', qc.get('user_code') == dev.get('user_code'), json.dumps(sorted(qc.keys())))
print('INFO complete-URI device page query keys: ' + json.dumps(sorted(qc.keys())))

st, dreq = call('GET', ADMIN + '/admin/oauth2/auth/requests/device?device_challenge=' + urllib.parse.quote(challenge or ''))
print('INFO device request (admin GET) status ' + str(st) + ' keys ' + (json.dumps(sorted(dreq.keys())) if isinstance(dreq, dict) else str(dreq)[:120]))

# ---- 3. accepting the user code ------------------------------------------------
st, bad = call('PUT', ADMIN + '/admin/oauth2/auth/requests/device/accept?device_challenge=' + urllib.parse.quote(challenge or ''), body={'user_code': 'WRONGCODE'})
check('a wrong user code is refused', st >= 400, f'{st} {bad}')
st, acc = call('PUT', ADMIN + '/admin/oauth2/auth/requests/device/accept?device_challenge=' + urllib.parse.quote(challenge or ''), body={'user_code': dev['user_code']})
check('the right user code is accepted with a redirect', st == 200 and isinstance(acc, dict) and 'redirect_to' in acc, f'{st} {acc}')

# ---- 4. then the ordinary login and consent requests -----------------------------
landed = browse(acc['redirect_to'])
check('after the code, the browser goes to the login UI with a login challenge', landed.startswith(UI + '/login') and 'login_challenge' in query(landed), landed)
login_challenge = query(landed).get('login_challenge')
st, lreq = call('GET', ADMIN + '/admin/oauth2/auth/requests/login?login_challenge=' + urllib.parse.quote(login_challenge))
print('INFO login request: client ' + str((lreq.get('client') or {}).get('client_id')) + ', request_url ' + str(lreq.get('request_url'))[:120] + ', has device_challenge: ' + str('device_challenge_id' in lreq or 'device' in json.dumps(lreq)[:2000]))
st, la = call('PUT', ADMIN + '/admin/oauth2/auth/requests/login/accept?login_challenge=' + urllib.parse.quote(login_challenge), body={'subject': 'user-1', 'remember': False})
landed = browse(la['redirect_to'])
check('then to consent', landed.startswith(UI + '/consent') and 'consent_challenge' in query(landed), landed)
consent_challenge = query(landed).get('consent_challenge')
st, creq = call('GET', ADMIN + '/admin/oauth2/auth/requests/consent?consent_challenge=' + urllib.parse.quote(consent_challenge))
st, ca = call('PUT', ADMIN + '/admin/oauth2/auth/requests/consent/accept?consent_challenge=' + urllib.parse.quote(consent_challenge), body={
    'grant_scope': creq.get('requested_scope'), 'session': {'access_token': {'tenant_id': 't-1'}, 'id_token': {'tenant_id': 't-1'}}})
landed = browse(ca['redirect_to'])
check('and ends on the configured device success page', landed.startswith(UI + '/device/done'), landed)
print('INFO success page query keys: ' + json.dumps(sorted(query(landed).keys())))

# ---- 5. the device gets its tokens -------------------------------------------------
st, tok = poll()
check('polling after approval returns tokens', st == 200, f'{st} {tok}')
if st == 200:
    print('INFO token response keys: ' + json.dumps(sorted(tok.keys())))
    check('an ID token and a refresh token are issued', 'id_token' in tok and 'refresh_token' in tok)
st, again = poll()
check('the device code works once', st == 400, f'{st} {again}')

print(f'\n{len(failures)} failed' if failures else '\nall checks passed')
raise SystemExit(1 if failures else 0)
