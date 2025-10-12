import React, { useState } from 'react'
import {
  BookOpenIcon,
  CodeBracketIcon,
  RocketLaunchIcon,
  KeyIcon,
  CubeIcon,
  CheckCircleIcon,
  ClipboardDocumentIcon,
} from '@heroicons/react/24/outline'
import { toast } from 'react-hot-toast'

const DocsPage: React.FC = () => {
  const [activeTab, setActiveTab] = useState<'overview' | 'quickstart' | 'integration' | 'api' | 'examples'>('overview')

  const copyToClipboard = (text: string, label: string) => {
    navigator.clipboard.writeText(text)
    toast.success(`${label} 복사됨`)
  }

  return (
    <div className="max-w-7xl mx-auto space-y-6">
      {/* 헤더 */}
      <div className="bg-gradient-to-r from-indigo-600 to-purple-600 rounded-lg shadow-lg p-8 text-white">
        <div className="flex items-center space-x-4">
          <BookOpenIcon className="h-12 w-12" />
          <div>
            <h1 className="text-3xl font-bold">개발자 가이드</h1>
            <p className="mt-2 text-indigo-100">
              Authway OAuth 2.0 / OpenID Connect 통합 문서
            </p>
          </div>
        </div>
      </div>

      {/* 탭 네비게이션 */}
      <div className="bg-white rounded-lg shadow">
        <div className="border-b border-gray-200">
          <nav className="flex -mb-px">
            {[
              { id: 'overview', name: '개요', icon: BookOpenIcon },
              { id: 'quickstart', name: '빠른 시작', icon: RocketLaunchIcon },
              { id: 'integration', name: '통합 가이드', icon: CodeBracketIcon },
              { id: 'api', name: 'API 레퍼런스', icon: KeyIcon },
              { id: 'examples', name: '예제', icon: CubeIcon },
            ].map((tab) => {
              const Icon = tab.icon
              return (
                <button
                  key={tab.id}
                  onClick={() => setActiveTab(tab.id as any)}
                  className={`
                    flex items-center space-x-2 px-6 py-4 text-sm font-medium border-b-2 transition-colors
                    ${
                      activeTab === tab.id
                        ? 'border-indigo-600 text-indigo-600'
                        : 'border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300'
                    }
                  `}
                >
                  <Icon className="h-5 w-5" />
                  <span>{tab.name}</span>
                </button>
              )
            })}
          </nav>
        </div>

        {/* 탭 콘텐츠 */}
        <div className="p-8">
          {activeTab === 'overview' && <OverviewTab />}
          {activeTab === 'quickstart' && <QuickStartTab copyToClipboard={copyToClipboard} />}
          {activeTab === 'integration' && <IntegrationTab copyToClipboard={copyToClipboard} />}
          {activeTab === 'api' && <APITab copyToClipboard={copyToClipboard} />}
          {activeTab === 'examples' && <ExamplesTab />}
        </div>
      </div>
    </div>
  )
}

const OverviewTab: React.FC = () => (
  <div className="space-y-6">
    <div>
      <h2 className="text-2xl font-bold text-gray-900">Authway란?</h2>
      <p className="mt-4 text-gray-600 leading-relaxed">
        Authway는 OAuth 2.0 및 OpenID Connect를 지원하는 중앙 인증 서버입니다.
        멀티 테넌트 아키텍처를 통해 여러 조직의 인증 요구사항을 단일 플랫폼에서 관리할 수 있습니다.
      </p>
    </div>

    <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
      <div className="bg-indigo-50 rounded-lg p-6">
        <h3 className="text-lg font-semibold text-indigo-900 mb-3">주요 기능</h3>
        <ul className="space-y-2 text-gray-700">
          <li className="flex items-start">
            <CheckCircleIcon className="h-5 w-5 text-indigo-600 mr-2 mt-0.5 flex-shrink-0" />
            <span>OAuth 2.0 Authorization Code Flow</span>
          </li>
          <li className="flex items-start">
            <CheckCircleIcon className="h-5 w-5 text-indigo-600 mr-2 mt-0.5 flex-shrink-0" />
            <span>OpenID Connect (OIDC) 지원</span>
          </li>
          <li className="flex items-start">
            <CheckCircleIcon className="h-5 w-5 text-indigo-600 mr-2 mt-0.5 flex-shrink-0" />
            <span>Single Sign-On (SSO)</span>
          </li>
          <li className="flex items-start">
            <CheckCircleIcon className="h-5 w-5 text-indigo-600 mr-2 mt-0.5 flex-shrink-0" />
            <span>멀티 테넌트 지원</span>
          </li>
          <li className="flex items-start">
            <CheckCircleIcon className="h-5 w-5 text-indigo-600 mr-2 mt-0.5 flex-shrink-0" />
            <span>Access Token + Refresh Token</span>
          </li>
          <li className="flex items-start">
            <CheckCircleIcon className="h-5 w-5 text-indigo-600 mr-2 mt-0.5 flex-shrink-0" />
            <span>Scope 기반 권한 관리</span>
          </li>
        </ul>
      </div>

      <div className="bg-purple-50 rounded-lg p-6">
        <h3 className="text-lg font-semibold text-purple-900 mb-3">지원하는 플로우</h3>
        <ul className="space-y-2 text-gray-700">
          <li className="flex items-start">
            <CheckCircleIcon className="h-5 w-5 text-purple-600 mr-2 mt-0.5 flex-shrink-0" />
            <span>Authorization Code Flow (권장)</span>
          </li>
          <li className="flex items-start">
            <CheckCircleIcon className="h-5 w-5 text-purple-600 mr-2 mt-0.5 flex-shrink-0" />
            <span>Refresh Token Flow</span>
          </li>
          <li className="flex items-start">
            <CheckCircleIcon className="h-5 w-5 text-purple-600 mr-2 mt-0.5 flex-shrink-0" />
            <span>Token Introspection</span>
          </li>
          <li className="flex items-start">
            <CheckCircleIcon className="h-5 w-5 text-purple-600 mr-2 mt-0.5 flex-shrink-0" />
            <span>Token Revocation</span>
          </li>
        </ul>
      </div>
    </div>

    <div className="bg-blue-50 border border-blue-200 rounded-lg p-6">
      <h3 className="text-lg font-semibold text-blue-900 mb-2">💡 시작하기 전에</h3>
      <p className="text-gray-700">
        Authway를 사용하려면 먼저 관리자 콘솔에서 테넌트와 OAuth 클라이언트를 등록해야 합니다.
        "빠른 시작" 탭에서 단계별 가이드를 확인하세요.
      </p>
    </div>
  </div>
)

const QuickStartTab: React.FC<{ copyToClipboard: (text: string, label: string) => void }> = ({ copyToClipboard }) => (
  <div className="space-y-6">
    <div>
      <h2 className="text-2xl font-bold text-gray-900">빠른 시작 가이드</h2>
      <p className="mt-2 text-gray-600">5분 안에 Authway를 앱에 통합하세요</p>
    </div>

    <div className="space-y-8">
      {/* Step 1 */}
      <div className="border-l-4 border-indigo-600 pl-6">
        <div className="flex items-center space-x-3 mb-3">
          <span className="flex items-center justify-center w-8 h-8 rounded-full bg-indigo-600 text-white font-bold">1</span>
          <h3 className="text-xl font-semibold text-gray-900">테넌트 생성</h3>
        </div>
        <p className="text-gray-600 mb-3">
          관리자 콘솔의 "테넌트 관리" 메뉴에서 새 테넌트를 생성합니다.
        </p>
        <div className="bg-gray-50 rounded p-4">
          <p className="text-sm text-gray-700">
            <strong>테넌트 ID</strong>는 나중에 클라이언트 등록 시 필요합니다.
          </p>
        </div>
      </div>

      {/* Step 2 */}
      <div className="border-l-4 border-indigo-600 pl-6">
        <div className="flex items-center space-x-3 mb-3">
          <span className="flex items-center justify-center w-8 h-8 rounded-full bg-indigo-600 text-white font-bold">2</span>
          <h3 className="text-xl font-semibold text-gray-900">OAuth 클라이언트 등록</h3>
        </div>
        <p className="text-gray-600 mb-3">
          "앱(클라이언트) 관리" 메뉴에서 OAuth 클라이언트를 등록합니다.
        </p>
        <div className="bg-gray-50 rounded p-4 space-y-2">
          <div>
            <strong className="text-sm text-gray-700">필수 정보:</strong>
            <ul className="mt-2 space-y-1 text-sm text-gray-600 ml-4">
              <li>• 클라이언트 이름</li>
              <li>• Redirect URI (콜백 URL)</li>
              <li>• Grant Types (authorization_code, refresh_token)</li>
              <li>• Scopes (openid, profile, email)</li>
            </ul>
          </div>
        </div>
      </div>

      {/* Step 3 */}
      <div className="border-l-4 border-indigo-600 pl-6">
        <div className="flex items-center space-x-3 mb-3">
          <span className="flex items-center justify-center w-8 h-8 rounded-full bg-indigo-600 text-white font-bold">3</span>
          <h3 className="text-xl font-semibold text-gray-900">환경 변수 설정</h3>
        </div>
        <p className="text-gray-600 mb-3">
          앱에서 다음 환경 변수를 설정합니다:
        </p>
        <CodeBlock
          language="bash"
          code={`AUTHWAY_URL=http://localhost:8080
CLIENT_ID=your-client-id
CLIENT_SECRET=your-client-secret
REDIRECT_URI=http://localhost:3000/callback`}
          onCopy={() => copyToClipboard(
            `AUTHWAY_URL=http://localhost:8080\nCLIENT_ID=your-client-id\nCLIENT_SECRET=your-client-secret\nREDIRECT_URI=http://localhost:3000/callback`,
            '환경 변수'
          )}
        />
      </div>

      {/* Step 4 */}
      <div className="border-l-4 border-indigo-600 pl-6">
        <div className="flex items-center space-x-3 mb-3">
          <span className="flex items-center justify-center w-8 h-8 rounded-full bg-indigo-600 text-white font-bold">4</span>
          <h3 className="text-xl font-semibold text-gray-900">OAuth 플로우 구현</h3>
        </div>
        <p className="text-gray-600 mb-3">
          "통합 가이드" 탭에서 언어별 구현 예제를 확인하세요.
        </p>
      </div>
    </div>
  </div>
)

const IntegrationTab: React.FC<{ copyToClipboard: (text: string, label: string) => void }> = ({ copyToClipboard }) => {
  const [language, setLanguage] = useState<'go' | 'nodejs' | 'python' | 'csharp'>('go')

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-bold text-gray-900">통합 가이드</h2>
        <p className="mt-2 text-gray-600">언어별 OAuth 2.0 통합 예제</p>
      </div>

      {/* 언어 선택 */}
      <div className="flex space-x-2 border-b border-gray-200">
        {[
          { id: 'go', name: 'Go' },
          { id: 'nodejs', name: 'Node.js' },
          { id: 'python', name: 'Python' },
          { id: 'csharp', name: 'C# ASP.NET' },
        ].map((lang) => (
          <button
            key={lang.id}
            onClick={() => setLanguage(lang.id as any)}
            className={`
              px-4 py-2 text-sm font-medium border-b-2 transition-colors
              ${
                language === lang.id
                  ? 'border-indigo-600 text-indigo-600'
                  : 'border-transparent text-gray-500 hover:text-gray-700'
              }
            `}
          >
            {lang.name}
          </button>
        ))}
      </div>

      {/* OAuth 플로우 다이어그램 */}
      <div className="bg-gray-50 rounded-lg p-6">
        <h3 className="text-lg font-semibold text-gray-900 mb-4">OAuth 2.0 Authorization Code Flow</h3>
        <pre className="text-sm text-gray-700 overflow-x-auto">
{`┌─────────┐                                      ┌──────────┐
│         │  1. Authorization Request             │          │
│  User   │───────────────────────────────────────>│ Authway  │
│ Browser │                                        │  Server  │
│         │  2. User Login & Authorization        │          │
│         │<───────────────────────────────────────│          │
│         │                                        │          │
│         │  3. Authorization Code                │          │
│         │<───────────────────────────────────────│          │
└────┬────┘                                        └────┬─────┘
     │                                                  │
     │ 4. Code → Your App                              │
     v                                                  │
┌─────────┐                                            │
│         │  5. Token Exchange                         │
│  Your   │────────────────────────────────────────────>
│  App    │                                            │
│         │  6. Access Token + Refresh Token          │
│         │<───────────────────────────────────────────│
│         │                                            │
│         │  7. Get User Info                          │
│         │────────────────────────────────────────────>
│         │                                            │
│         │  8. User Profile                           │
│         │<───────────────────────────────────────────│
└─────────┘                                            └─────────┘`}
        </pre>
      </div>

      {/* 코드 예제 */}
      {language === 'go' && <GoExample copyToClipboard={copyToClipboard} />}
      {language === 'nodejs' && <NodeJSExample copyToClipboard={copyToClipboard} />}
      {language === 'python' && <PythonExample copyToClipboard={copyToClipboard} />}
      {language === 'csharp' && <CSharpExample copyToClipboard={copyToClipboard} />}
    </div>
  )
}

const GoExample: React.FC<{ copyToClipboard: (text: string, label: string) => void }> = ({ copyToClipboard }) => (
  <div className="space-y-6">
    <div>
      <h3 className="text-lg font-semibold text-gray-900 mb-3">1. 의존성 설치</h3>
      <CodeBlock
        language="bash"
        code={`go get golang.org/x/oauth2`}
        onCopy={() => copyToClipboard('go get golang.org/x/oauth2', '명령어')}
      />
    </div>

    <div>
      <h3 className="text-lg font-semibold text-gray-900 mb-3">2. OAuth 설정</h3>
      <CodeBlock
        language="go"
        code={`package main

import (
    "golang.org/x/oauth2"
)

var oauthConfig = &oauth2.Config{
    ClientID:     "your-client-id",
    ClientSecret: "your-client-secret",
    RedirectURL:  "http://localhost:3000/callback",
    Scopes:       []string{"openid", "profile", "email"},
    Endpoint: oauth2.Endpoint{
        AuthURL:  "http://localhost:8080/oauth/authorize",
        TokenURL: "http://localhost:8080/oauth/token",
    },
}`}
        onCopy={() => copyToClipboard(`package main

import (
    "golang.org/x/oauth2"
)

var oauthConfig = &oauth2.Config{
    ClientID:     "your-client-id",
    ClientSecret: "your-client-secret",
    RedirectURL:  "http://localhost:3000/callback",
    Scopes:       []string{"openid", "profile", "email"},
    Endpoint: oauth2.Endpoint{
        AuthURL:  "http://localhost:8080/oauth/authorize",
        TokenURL: "http://localhost:8080/oauth/token",
    },
}`, 'Go 코드')}
      />
    </div>

    <div>
      <h3 className="text-lg font-semibold text-gray-900 mb-3">3. 로그인 핸들러</h3>
      <CodeBlock
        language="go"
        code={`func handleLogin(w http.ResponseWriter, r *http.Request) {
    // Generate state for CSRF protection
    state := generateRandomState()

    // Store state in session/cookie
    http.SetCookie(w, &http.Cookie{
        Name:     "oauth_state",
        Value:    state,
        MaxAge:   300,
        HttpOnly: true,
    })

    // Redirect to authorization URL
    url := oauthConfig.AuthCodeURL(state)
    http.Redirect(w, r, url, http.StatusFound)
}`}
        onCopy={() => copyToClipboard(`func handleLogin(w http.ResponseWriter, r *http.Request) {
    state := generateRandomState()
    http.SetCookie(w, &http.Cookie{
        Name:     "oauth_state",
        Value:    state,
        MaxAge:   300,
        HttpOnly: true,
    })
    url := oauthConfig.AuthCodeURL(state)
    http.Redirect(w, r, url, http.StatusFound)
}`, 'Go 코드')}
      />
    </div>

    <div>
      <h3 className="text-lg font-semibold text-gray-900 mb-3">4. 콜백 핸들러</h3>
      <CodeBlock
        language="go"
        code={`func handleCallback(w http.ResponseWriter, r *http.Request) {
    // Verify state
    stateCookie, _ := r.Cookie("oauth_state")
    if r.URL.Query().Get("state") != stateCookie.Value {
        http.Error(w, "Invalid state", http.StatusBadRequest)
        return
    }

    // Exchange code for token
    code := r.URL.Query().Get("code")
    token, err := oauthConfig.Exchange(r.Context(), code)
    if err != nil {
        http.Error(w, "Token exchange failed", http.StatusInternalServerError)
        return
    }

    // Get user info
    client := oauthConfig.Client(r.Context(), token)
    resp, err := client.Get("http://localhost:8080/oauth/userinfo")
    if err != nil {
        http.Error(w, "Failed to get user info", http.StatusInternalServerError)
        return
    }
    defer resp.Body.Close()

    // Parse user info and create session
    // ... (store token and user info in session)

    http.Redirect(w, r, "/", http.StatusFound)
}`}
        onCopy={() => copyToClipboard(`func handleCallback(w http.ResponseWriter, r *http.Request) {
    stateCookie, _ := r.Cookie("oauth_state")
    if r.URL.Query().Get("state") != stateCookie.Value {
        http.Error(w, "Invalid state", http.StatusBadRequest)
        return
    }
    code := r.URL.Query().Get("code")
    token, err := oauthConfig.Exchange(r.Context(), code)
    if err != nil {
        http.Error(w, "Token exchange failed", http.StatusInternalServerError)
        return
    }
    client := oauthConfig.Client(r.Context(), token)
    resp, err := client.Get("http://localhost:8080/oauth/userinfo")
    if err != nil {
        http.Error(w, "Failed to get user info", http.StatusInternalServerError)
        return
    }
    defer resp.Body.Close()
    http.Redirect(w, r, "/", http.StatusFound)
}`, 'Go 코드')}
      />
    </div>
  </div>
)

const NodeJSExample: React.FC<{ copyToClipboard: (text: string, label: string) => void }> = ({ copyToClipboard }) => (
  <div className="space-y-6">
    <div>
      <h3 className="text-lg font-semibold text-gray-900 mb-3">1. 의존성 설치</h3>
      <CodeBlock
        language="bash"
        code={`npm install openid-client express express-session`}
        onCopy={() => copyToClipboard('npm install openid-client express express-session', '명령어')}
      />
    </div>

    <div>
      <h3 className="text-lg font-semibold text-gray-900 mb-3">2. OAuth 설정</h3>
      <CodeBlock
        language="javascript"
        code={`const { Issuer, generators } = require('openid-client');
const express = require('express');
const session = require('express-session');

const app = express();

app.use(session({
  secret: 'your-secret-key',
  resave: false,
  saveUninitialized: false,
}));

let client;

async function setupOAuth() {
  const issuer = await Issuer.discover('http://localhost:8080');

  client = new issuer.Client({
    client_id: 'your-client-id',
    client_secret: 'your-client-secret',
    redirect_uris: ['http://localhost:3000/callback'],
    response_types: ['code'],
  });
}

setupOAuth();`}
        onCopy={() => copyToClipboard(`const { Issuer, generators } = require('openid-client');
const express = require('express');
const session = require('express-session');

const app = express();

app.use(session({
  secret: 'your-secret-key',
  resave: false,
  saveUninitialized: false,
}));

let client;

async function setupOAuth() {
  const issuer = await Issuer.discover('http://localhost:8080');

  client = new issuer.Client({
    client_id: 'your-client-id',
    client_secret: 'your-client-secret',
    redirect_uris: ['http://localhost:3000/callback'],
    response_types: ['code'],
  });
}

setupOAuth();`, 'Node.js 코드')}
      />
    </div>

    <div>
      <h3 className="text-lg font-semibold text-gray-900 mb-3">3. 로그인 라우트</h3>
      <CodeBlock
        language="javascript"
        code={`app.get('/login', (req, res) => {
  const state = generators.state();
  const nonce = generators.nonce();

  req.session.state = state;
  req.session.nonce = nonce;

  const authUrl = client.authorizationUrl({
    scope: 'openid profile email',
    state: state,
    nonce: nonce,
  });

  res.redirect(authUrl);
});`}
        onCopy={() => copyToClipboard(`app.get('/login', (req, res) => {
  const state = generators.state();
  const nonce = generators.nonce();

  req.session.state = state;
  req.session.nonce = nonce;

  const authUrl = client.authorizationUrl({
    scope: 'openid profile email',
    state: state,
    nonce: nonce,
  });

  res.redirect(authUrl);
});`, 'Node.js 코드')}
      />
    </div>

    <div>
      <h3 className="text-lg font-semibold text-gray-900 mb-3">4. 콜백 라우트</h3>
      <CodeBlock
        language="javascript"
        code={`app.get('/callback', async (req, res) => {
  const params = client.callbackParams(req);

  try {
    const tokenSet = await client.callback(
      'http://localhost:3000/callback',
      params,
      { state: req.session.state, nonce: req.session.nonce }
    );

    const userinfo = await client.userinfo(tokenSet.access_token);

    req.session.user = userinfo;
    req.session.tokens = tokenSet;

    res.redirect('/');
  } catch (err) {
    console.error('OAuth callback error:', err);
    res.status(500).send('Authentication failed');
  }
});

app.listen(3000, () => {
  console.log('Server running on http://localhost:3000');
});`}
        onCopy={() => copyToClipboard(`app.get('/callback', async (req, res) => {
  const params = client.callbackParams(req);

  try {
    const tokenSet = await client.callback(
      'http://localhost:3000/callback',
      params,
      { state: req.session.state, nonce: req.session.nonce }
    );

    const userinfo = await client.userinfo(tokenSet.access_token);

    req.session.user = userinfo;
    req.session.tokens = tokenSet;

    res.redirect('/');
  } catch (err) {
    console.error('OAuth callback error:', err);
    res.status(500).send('Authentication failed');
  }
});

app.listen(3000, () => {
  console.log('Server running on http://localhost:3000');
});`, 'Node.js 코드')}
      />
    </div>
  </div>
)

const PythonExample: React.FC<{ copyToClipboard: (text: string, label: string) => void }> = ({ copyToClipboard }) => (
  <div className="space-y-6">
    <div>
      <h3 className="text-lg font-semibold text-gray-900 mb-3">1. 의존성 설치</h3>
      <CodeBlock
        language="bash"
        code={`pip install authlib flask requests`}
        onCopy={() => copyToClipboard('pip install authlib flask requests', '명령어')}
      />
    </div>

    <div>
      <h3 className="text-lg font-semibold text-gray-900 mb-3">2. OAuth 설정</h3>
      <CodeBlock
        language="python"
        code={`from flask import Flask, session, redirect, url_for, request
from authlib.integrations.flask_client import OAuth
import secrets

app = Flask(__name__)
app.secret_key = secrets.token_hex(16)

oauth = OAuth(app)

oauth.register(
    name='authway',
    client_id='your-client-id',
    client_secret='your-client-secret',
    server_metadata_url='http://localhost:8080/.well-known/openid-configuration',
    client_kwargs={
        'scope': 'openid profile email'
    }
)`}
        onCopy={() => copyToClipboard(`from flask import Flask, session, redirect, url_for, request
from authlib.integrations.flask_client import OAuth
import secrets

app = Flask(__name__)
app.secret_key = secrets.token_hex(16)

oauth = OAuth(app)

oauth.register(
    name='authway',
    client_id='your-client-id',
    client_secret='your-client-secret',
    server_metadata_url='http://localhost:8080/.well-known/openid-configuration',
    client_kwargs={
        'scope': 'openid profile email'
    }
)`, 'Python 코드')}
      />
    </div>

    <div>
      <h3 className="text-lg font-semibold text-gray-900 mb-3">3. 로그인 라우트</h3>
      <CodeBlock
        language="python"
        code={`@app.route('/login')
def login():
    redirect_uri = url_for('callback', _external=True)
    return oauth.authway.authorize_redirect(redirect_uri)`}
        onCopy={() => copyToClipboard(`@app.route('/login')
def login():
    redirect_uri = url_for('callback', _external=True)
    return oauth.authway.authorize_redirect(redirect_uri)`, 'Python 코드')}
      />
    </div>

    <div>
      <h3 className="text-lg font-semibold text-gray-900 mb-3">4. 콜백 라우트</h3>
      <CodeBlock
        language="python"
        code={`@app.route('/callback')
def callback():
    try:
        token = oauth.authway.authorize_access_token()
        userinfo = token.get('userinfo')

        session['user'] = userinfo
        session['token'] = token

        return redirect('/')
    except Exception as e:
        print(f'OAuth callback error: {e}')
        return 'Authentication failed', 500

if __name__ == '__main__':
    app.run(port=3000, debug=True)`}
        onCopy={() => copyToClipboard(`@app.route('/callback')
def callback():
    try:
        token = oauth.authway.authorize_access_token()
        userinfo = token.get('userinfo')

        session['user'] = userinfo
        session['token'] = token

        return redirect('/')
    except Exception as e:
        print(f'OAuth callback error: {e}')
        return 'Authentication failed', 500

if __name__ == '__main__':
    app.run(port=3000, debug=True)`, 'Python 코드')}
      />
    </div>
  </div>
)

const CSharpExample: React.FC<{ copyToClipboard: (text: string, label: string) => void }> = ({ copyToClipboard }) => (
  <div className="space-y-6">
    <div>
      <h3 className="text-lg font-semibold text-gray-900 mb-3">1. 패키지 설치</h3>
      <CodeBlock
        language="bash"
        code={`dotnet add package Microsoft.AspNetCore.Authentication.OpenIdConnect
dotnet add package Microsoft.AspNetCore.Authentication.Cookies`}
        onCopy={() => copyToClipboard('dotnet add package Microsoft.AspNetCore.Authentication.OpenIdConnect\ndotnet add package Microsoft.AspNetCore.Authentication.Cookies', '명령어')}
      />
    </div>

    <div>
      <h3 className="text-lg font-semibold text-gray-900 mb-3">2. appsettings.json 설정</h3>
      <CodeBlock
        language="json"
        code={`{
  "Authentication": {
    "Authway": {
      "Authority": "http://localhost:8080",
      "ClientId": "your-client-id",
      "ClientSecret": "your-client-secret",
      "ResponseType": "code",
      "SaveTokens": true,
      "GetClaimsFromUserInfoEndpoint": true,
      "Scope": ["openid", "profile", "email"]
    }
  }
}`}
        onCopy={() => copyToClipboard(`{
  "Authentication": {
    "Authway": {
      "Authority": "http://localhost:8080",
      "ClientId": "your-client-id",
      "ClientSecret": "your-client-secret",
      "ResponseType": "code",
      "SaveTokens": true,
      "GetClaimsFromUserInfoEndpoint": true,
      "Scope": ["openid", "profile", "email"]
    }
  }
}`, 'JSON 설정')}
      />
    </div>

    <div>
      <h3 className="text-lg font-semibold text-gray-900 mb-3">3. Program.cs 설정</h3>
      <CodeBlock
        language="csharp"
        code={`using Microsoft.AspNetCore.Authentication.Cookies;
using Microsoft.AspNetCore.Authentication.OpenIdConnect;

var builder = WebApplication.CreateBuilder(args);

// Add services to the container
builder.Services.AddControllersWithViews();

// Configure Authentication
builder.Services.AddAuthentication(options =>
{
    options.DefaultScheme = CookieAuthenticationDefaults.AuthenticationScheme;
    options.DefaultChallengeScheme = OpenIdConnectDefaults.AuthenticationScheme;
})
.AddCookie()
.AddOpenIdConnect("Authway", options =>
{
    var authConfig = builder.Configuration.GetSection("Authentication:Authway");

    options.Authority = authConfig["Authority"];
    options.ClientId = authConfig["ClientId"];
    options.ClientSecret = authConfig["ClientSecret"];
    options.ResponseType = authConfig["ResponseType"];
    options.SaveTokens = bool.Parse(authConfig["SaveTokens"]);
    options.GetClaimsFromUserInfoEndpoint =
        bool.Parse(authConfig["GetClaimsFromUserInfoEndpoint"]);

    options.CallbackPath = "/signin-oidc";

    // Add scopes
    options.Scope.Clear();
    foreach (var scope in authConfig.GetSection("Scope").Get<string[]>())
    {
        options.Scope.Add(scope);
    }
});

var app = builder.Build();

// Configure the HTTP request pipeline
if (!app.Environment.IsDevelopment())
{
    app.UseExceptionHandler("/Home/Error");
}

app.UseStaticFiles();
app.UseRouting();

app.UseAuthentication();
app.UseAuthorization();

app.MapControllerRoute(
    name: "default",
    pattern: "{controller=Home}/{action=Index}/{id?}");

app.Run();`}
        onCopy={() => copyToClipboard(`using Microsoft.AspNetCore.Authentication.Cookies;
using Microsoft.AspNetCore.Authentication.OpenIdConnect;

var builder = WebApplication.CreateBuilder(args);

builder.Services.AddControllersWithViews();

builder.Services.AddAuthentication(options =>
{
    options.DefaultScheme = CookieAuthenticationDefaults.AuthenticationScheme;
    options.DefaultChallengeScheme = OpenIdConnectDefaults.AuthenticationScheme;
})
.AddCookie()
.AddOpenIdConnect("Authway", options =>
{
    var authConfig = builder.Configuration.GetSection("Authentication:Authway");

    options.Authority = authConfig["Authority"];
    options.ClientId = authConfig["ClientId"];
    options.ClientSecret = authConfig["ClientSecret"];
    options.ResponseType = authConfig["ResponseType"];
    options.SaveTokens = bool.Parse(authConfig["SaveTokens"]);
    options.GetClaimsFromUserInfoEndpoint =
        bool.Parse(authConfig["GetClaimsFromUserInfoEndpoint"]);

    options.CallbackPath = "/signin-oidc";

    options.Scope.Clear();
    foreach (var scope in authConfig.GetSection("Scope").Get<string[]>())
    {
        options.Scope.Add(scope);
    }
});

var app = builder.Build();

if (!app.Environment.IsDevelopment())
{
    app.UseExceptionHandler("/Home/Error");
}

app.UseStaticFiles();
app.UseRouting();

app.UseAuthentication();
app.UseAuthorization();

app.MapControllerRoute(
    name: "default",
    pattern: "{controller=Home}/{action=Index}/{id?}");

app.Run();`, 'C# 코드')}
      />
    </div>

    <div>
      <h3 className="text-lg font-semibold text-gray-900 mb-3">4. 컨트롤러 예제</h3>
      <CodeBlock
        language="csharp"
        code={`using Microsoft.AspNetCore.Authentication;
using Microsoft.AspNetCore.Authentication.Cookies;
using Microsoft.AspNetCore.Authentication.OpenIdConnect;
using Microsoft.AspNetCore.Authorization;
using Microsoft.AspNetCore.Mvc;

namespace YourApp.Controllers
{
    public class AccountController : Controller
    {
        [HttpGet]
        public IActionResult Login(string returnUrl = "/")
        {
            var properties = new AuthenticationProperties
            {
                RedirectUri = returnUrl
            };

            return Challenge(properties, OpenIdConnectDefaults.AuthenticationScheme);
        }

        [Authorize]
        [HttpGet]
        public async Task<IActionResult> Logout()
        {
            // Sign out from cookie authentication
            await HttpContext.SignOutAsync(
                CookieAuthenticationDefaults.AuthenticationScheme);

            // Sign out from OpenID Connect
            await HttpContext.SignOutAsync(
                OpenIdConnectDefaults.AuthenticationScheme);

            return RedirectToAction("Index", "Home");
        }

        [Authorize]
        [HttpGet]
        public IActionResult Profile()
        {
            // Access user claims
            var userId = User.FindFirst("sub")?.Value;
            var userName = User.FindFirst("name")?.Value;
            var userEmail = User.FindFirst("email")?.Value;

            ViewData["UserId"] = userId;
            ViewData["UserName"] = userName;
            ViewData["UserEmail"] = userEmail;

            return View();
        }
    }
}`}
        onCopy={() => copyToClipboard(`using Microsoft.AspNetCore.Authentication;
using Microsoft.AspNetCore.Authentication.Cookies;
using Microsoft.AspNetCore.Authentication.OpenIdConnect;
using Microsoft.AspNetCore.Authorization;
using Microsoft.AspNetCore.Mvc;

namespace YourApp.Controllers
{
    public class AccountController : Controller
    {
        [HttpGet]
        public IActionResult Login(string returnUrl = "/")
        {
            var properties = new AuthenticationProperties
            {
                RedirectUri = returnUrl
            };

            return Challenge(properties, OpenIdConnectDefaults.AuthenticationScheme);
        }

        [Authorize]
        [HttpGet]
        public async Task<IActionResult> Logout()
        {
            await HttpContext.SignOutAsync(
                CookieAuthenticationDefaults.AuthenticationScheme);

            await HttpContext.SignOutAsync(
                OpenIdConnectDefaults.AuthenticationScheme);

            return RedirectToAction("Index", "Home");
        }

        [Authorize]
        [HttpGet]
        public IActionResult Profile()
        {
            var userId = User.FindFirst("sub")?.Value;
            var userName = User.FindFirst("name")?.Value;
            var userEmail = User.FindFirst("email")?.Value;

            ViewData["UserId"] = userId;
            ViewData["UserName"] = userName;
            ViewData["UserEmail"] = userEmail;

            return View();
        }
    }
}`, 'C# 코드')}
      />
    </div>

    <div>
      <h3 className="text-lg font-semibold text-gray-900 mb-3">5. Access Token 사용</h3>
      <CodeBlock
        language="csharp"
        code={`[Authorize]
[HttpGet]
public async Task<IActionResult> CallApi()
{
    // Get access token from authentication properties
    var accessToken = await HttpContext.GetTokenAsync("access_token");

    if (string.IsNullOrEmpty(accessToken))
    {
        return Unauthorized();
    }

    // Use access token to call API
    using var client = new HttpClient();
    client.DefaultRequestHeaders.Authorization =
        new System.Net.Http.Headers.AuthenticationHeaderValue("Bearer", accessToken);

    var response = await client.GetAsync("http://localhost:8080/oauth/userinfo");

    if (response.IsSuccessStatusCode)
    {
        var userInfo = await response.Content.ReadAsStringAsync();
        return Content(userInfo, "application/json");
    }

    return StatusCode((int)response.StatusCode);
}`}
        onCopy={() => copyToClipboard(`[Authorize]
[HttpGet]
public async Task<IActionResult> CallApi()
{
    var accessToken = await HttpContext.GetTokenAsync("access_token");

    if (string.IsNullOrEmpty(accessToken))
    {
        return Unauthorized();
    }

    using var client = new HttpClient();
    client.DefaultRequestHeaders.Authorization =
        new System.Net.Http.Headers.AuthenticationHeaderValue("Bearer", accessToken);

    var response = await client.GetAsync("http://localhost:8080/oauth/userinfo");

    if (response.IsSuccessStatusCode)
    {
        var userInfo = await response.Content.ReadAsStringAsync();
        return Content(userInfo, "application/json");
    }

    return StatusCode((int)response.StatusCode);
}`, 'C# 코드')}
      />
    </div>
  </div>
)

const APITab: React.FC<{ copyToClipboard: (text: string, label: string) => void }> = ({ copyToClipboard }) => (
  <div className="space-y-6">
    <div>
      <h2 className="text-2xl font-bold text-gray-900">API 레퍼런스</h2>
      <p className="mt-2 text-gray-600">Authway OAuth 2.0 / OIDC 엔드포인트</p>
    </div>

    {/* Authorization Endpoint */}
    <div className="bg-white border border-gray-200 rounded-lg p-6">
      <div className="flex items-center justify-between mb-4">
        <h3 className="text-lg font-semibold text-gray-900">Authorization</h3>
        <span className="px-3 py-1 text-xs font-semibold bg-blue-100 text-blue-800 rounded-full">GET</span>
      </div>
      <CodeBlock
        language="http"
        code={`GET /oauth/authorize?
  response_type=code&
  client_id=your-client-id&
  redirect_uri=http://localhost:3000/callback&
  scope=openid profile email&
  state=random-state-string`}
        onCopy={() => copyToClipboard('GET /oauth/authorize?response_type=code&client_id=your-client-id&redirect_uri=http://localhost:3000/callback&scope=openid profile email&state=random-state-string', 'API URL')}
      />
      <div className="mt-4">
        <h4 className="font-medium text-gray-900 mb-2">Parameters:</h4>
        <ul className="space-y-1 text-sm text-gray-600">
          <li><code className="bg-gray-100 px-1 rounded">response_type</code> - "code" (필수)</li>
          <li><code className="bg-gray-100 px-1 rounded">client_id</code> - 클라이언트 ID (필수)</li>
          <li><code className="bg-gray-100 px-1 rounded">redirect_uri</code> - 콜백 URL (필수)</li>
          <li><code className="bg-gray-100 px-1 rounded">scope</code> - 요청 권한 (예: "openid profile email")</li>
          <li><code className="bg-gray-100 px-1 rounded">state</code> - CSRF 보호용 랜덤 문자열 (권장)</li>
        </ul>
      </div>
    </div>

    {/* Token Endpoint */}
    <div className="bg-white border border-gray-200 rounded-lg p-6">
      <div className="flex items-center justify-between mb-4">
        <h3 className="text-lg font-semibold text-gray-900">Token Exchange</h3>
        <span className="px-3 py-1 text-xs font-semibold bg-green-100 text-green-800 rounded-full">POST</span>
      </div>
      <CodeBlock
        language="http"
        code={`POST /oauth/token
Content-Type: application/x-www-form-urlencoded

grant_type=authorization_code&
code=authorization-code&
redirect_uri=http://localhost:3000/callback&
client_id=your-client-id&
client_secret=your-client-secret`}
        onCopy={() => copyToClipboard(`POST /oauth/token
Content-Type: application/x-www-form-urlencoded

grant_type=authorization_code&
code=authorization-code&
redirect_uri=http://localhost:3000/callback&
client_id=your-client-id&
client_secret=your-client-secret`, 'API Request')}
      />
      <div className="mt-4">
        <h4 className="font-medium text-gray-900 mb-2">Response:</h4>
        <CodeBlock
          language="json"
          code={`{
  "access_token": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...",
  "token_type": "Bearer",
  "expires_in": 3600,
  "refresh_token": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...",
  "scope": "openid profile email",
  "id_token": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9..."
}`}
          onCopy={() => copyToClipboard(`{
  "access_token": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...",
  "token_type": "Bearer",
  "expires_in": 3600,
  "refresh_token": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...",
  "scope": "openid profile email",
  "id_token": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9..."
}`, 'JSON Response')}
        />
      </div>
    </div>

    {/* UserInfo Endpoint */}
    <div className="bg-white border border-gray-200 rounded-lg p-6">
      <div className="flex items-center justify-between mb-4">
        <h3 className="text-lg font-semibold text-gray-900">User Info</h3>
        <span className="px-3 py-1 text-xs font-semibold bg-blue-100 text-blue-800 rounded-full">GET</span>
      </div>
      <CodeBlock
        language="http"
        code={`GET /oauth/userinfo
Authorization: Bearer access-token`}
        onCopy={() => copyToClipboard('GET /oauth/userinfo\nAuthorization: Bearer access-token', 'API Request')}
      />
      <div className="mt-4">
        <h4 className="font-medium text-gray-900 mb-2">Response:</h4>
        <CodeBlock
          language="json"
          code={`{
  "sub": "user-unique-id",
  "email": "user@example.com",
  "email_verified": true,
  "name": "User Name",
  "picture": "https://example.com/avatar.jpg"
}`}
          onCopy={() => copyToClipboard(`{
  "sub": "user-unique-id",
  "email": "user@example.com",
  "email_verified": true,
  "name": "User Name",
  "picture": "https://example.com/avatar.jpg"
}`, 'JSON Response')}
        />
      </div>
    </div>

    {/* Refresh Token */}
    <div className="bg-white border border-gray-200 rounded-lg p-6">
      <div className="flex items-center justify-between mb-4">
        <h3 className="text-lg font-semibold text-gray-900">Refresh Token</h3>
        <span className="px-3 py-1 text-xs font-semibold bg-green-100 text-green-800 rounded-full">POST</span>
      </div>
      <CodeBlock
        language="http"
        code={`POST /oauth/token
Content-Type: application/x-www-form-urlencoded

grant_type=refresh_token&
refresh_token=refresh-token&
client_id=your-client-id&
client_secret=your-client-secret`}
        onCopy={() => copyToClipboard(`POST /oauth/token
Content-Type: application/x-www-form-urlencoded

grant_type=refresh_token&
refresh_token=refresh-token&
client_id=your-client-id&
client_secret=your-client-secret`, 'API Request')}
      />
    </div>

    {/* Token Revocation */}
    <div className="bg-white border border-gray-200 rounded-lg p-6">
      <div className="flex items-center justify-between mb-4">
        <h3 className="text-lg font-semibold text-gray-900">Token Revocation</h3>
        <span className="px-3 py-1 text-xs font-semibold bg-green-100 text-green-800 rounded-full">POST</span>
      </div>
      <CodeBlock
        language="http"
        code={`POST /oauth/revoke
Content-Type: application/x-www-form-urlencoded

token=token-to-revoke&
client_id=your-client-id&
client_secret=your-client-secret`}
        onCopy={() => copyToClipboard(`POST /oauth/revoke
Content-Type: application/x-www-form-urlencoded

token=token-to-revoke&
client_id=your-client-id&
client_secret=your-client-secret`, 'API Request')}
      />
    </div>
  </div>
)

const ExamplesTab: React.FC = () => (
  <div className="space-y-6">
    <div>
      <h2 className="text-2xl font-bold text-gray-900">예제 프로젝트</h2>
      <p className="mt-2 text-gray-600">실제 작동하는 샘플 앱을 확인하세요</p>
    </div>

    <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
      {/* Apple Service */}
      <div className="bg-gradient-to-br from-red-50 to-red-100 border border-red-200 rounded-lg p-6">
        <div className="text-4xl mb-4">🍎</div>
        <h3 className="text-lg font-semibold text-gray-900 mb-2">Apple Service</h3>
        <p className="text-sm text-gray-600 mb-4">
          Go로 작성된 기본 OAuth 2.0 클라이언트 예제
        </p>
        <div className="space-y-2 text-sm">
          <div className="flex items-center text-gray-700">
            <span className="font-medium w-16">포트:</span>
            <code className="bg-white/50 px-2 py-0.5 rounded">9001</code>
          </div>
          <div className="flex items-center text-gray-700">
            <span className="font-medium w-16">URL:</span>
            <a href="http://localhost:9001" target="_blank" rel="noopener noreferrer" className="text-indigo-600 hover:underline">
              localhost:9001
            </a>
          </div>
        </div>
        <div className="mt-4 pt-4 border-t border-red-200">
          <p className="text-xs text-gray-600">
            <strong>위치:</strong> <code>samples/AppleService/</code>
          </p>
        </div>
      </div>

      {/* Banana Service */}
      <div className="bg-gradient-to-br from-yellow-50 to-yellow-100 border border-yellow-200 rounded-lg p-6">
        <div className="text-4xl mb-4">🍌</div>
        <h3 className="text-lg font-semibold text-gray-900 mb-2">Banana Service</h3>
        <p className="text-sm text-gray-600 mb-4">
          SSO 테스트를 위한 두 번째 샘플 앱
        </p>
        <div className="space-y-2 text-sm">
          <div className="flex items-center text-gray-700">
            <span className="font-medium w-16">포트:</span>
            <code className="bg-white/50 px-2 py-0.5 rounded">9002</code>
          </div>
          <div className="flex items-center text-gray-700">
            <span className="font-medium w-16">URL:</span>
            <a href="http://localhost:9002" target="_blank" rel="noopener noreferrer" className="text-indigo-600 hover:underline">
              localhost:9002
            </a>
          </div>
        </div>
        <div className="mt-4 pt-4 border-t border-yellow-200">
          <p className="text-xs text-gray-600">
            <strong>위치:</strong> <code>samples/BananaService/</code>
          </p>
        </div>
      </div>

      {/* Chocolate Service */}
      <div className="bg-gradient-to-br from-amber-50 to-amber-100 border border-amber-200 rounded-lg p-6">
        <div className="text-4xl mb-4">🍫</div>
        <h3 className="text-lg font-semibold text-gray-900 mb-2">Chocolate Service</h3>
        <p className="text-sm text-gray-600 mb-4">
          멀티 서비스 SSO 테스트용 세 번째 앱
        </p>
        <div className="space-y-2 text-sm">
          <div className="flex items-center text-gray-700">
            <span className="font-medium w-16">포트:</span>
            <code className="bg-white/50 px-2 py-0.5 rounded">9003</code>
          </div>
          <div className="flex items-center text-gray-700">
            <span className="font-medium w-16">URL:</span>
            <a href="http://localhost:9003" target="_blank" rel="noopener noreferrer" className="text-indigo-600 hover:underline">
              localhost:9003
            </a>
          </div>
        </div>
        <div className="mt-4 pt-4 border-t border-amber-200">
          <p className="text-xs text-gray-600">
            <strong>위치:</strong> <code>samples/ChocolateService/</code>
          </p>
        </div>
      </div>
    </div>

    {/* 샘플 실행 방법 */}
    <div className="bg-blue-50 border border-blue-200 rounded-lg p-6">
      <h3 className="text-lg font-semibold text-blue-900 mb-4">샘플 앱 실행 방법</h3>
      <div className="space-y-4">
        <div>
          <h4 className="font-medium text-gray-900 mb-2">1. 클라이언트 등록</h4>
          <CodeBlock
            language="bash"
            code={`cd samples
.\\setup-clients.ps1  # Windows
# or
./setup-clients.sh   # Linux/Mac`}
            onCopy={() => {}}
          />
        </div>
        <div>
          <h4 className="font-medium text-gray-900 mb-2">2. 샘플 서비스 시작</h4>
          <CodeBlock
            language="bash"
            code={`cd samples/AppleService
go run main.go

# 다른 터미널에서
cd samples/BananaService
go run main.go

# 또 다른 터미널에서
cd samples/ChocolateService
go run main.go`}
            onCopy={() => {}}
          />
        </div>
        <div>
          <h4 className="font-medium text-gray-900 mb-2">3. 브라우저에서 테스트</h4>
          <p className="text-sm text-gray-700">
            각 서비스에 접속하여 로그인 후 SSO가 작동하는지 확인하세요:
          </p>
          <ul className="mt-2 space-y-1 text-sm text-gray-600">
            <li>• http://localhost:9001</li>
            <li>• http://localhost:9002</li>
            <li>• http://localhost:9003</li>
          </ul>
        </div>
      </div>
    </div>

    {/* 테스트 시나리오 */}
    <div className="bg-white border border-gray-200 rounded-lg p-6">
      <h3 className="text-lg font-semibold text-gray-900 mb-4">테스트 시나리오</h3>
      <div className="space-y-4">
        <div className="flex items-start space-x-3">
          <CheckCircleIcon className="h-6 w-6 text-green-500 flex-shrink-0 mt-0.5" />
          <div>
            <h4 className="font-medium text-gray-900">기본 OAuth Flow</h4>
            <p className="text-sm text-gray-600">Apple Service에서 로그인하여 인증 플로우 테스트</p>
          </div>
        </div>
        <div className="flex items-start space-x-3">
          <CheckCircleIcon className="h-6 w-6 text-green-500 flex-shrink-0 mt-0.5" />
          <div>
            <h4 className="font-medium text-gray-900">Single Sign-On (SSO)</h4>
            <p className="text-sm text-gray-600">Apple Service 로그인 후 Banana Service에서 자동 로그인 확인</p>
          </div>
        </div>
        <div className="flex items-start space-x-3">
          <CheckCircleIcon className="h-6 w-6 text-green-500 flex-shrink-0 mt-0.5" />
          <div>
            <h4 className="font-medium text-gray-900">멀티 서비스 세션</h4>
            <p className="text-sm text-gray-600">3개 서비스 모두 로그인 후 세션 관리 테스트</p>
          </div>
        </div>
        <div className="flex items-start space-x-3">
          <CheckCircleIcon className="h-6 w-6 text-green-500 flex-shrink-0 mt-0.5" />
          <div>
            <h4 className="font-medium text-gray-900">토큰 갱신</h4>
            <p className="text-sm text-gray-600">Access Token 만료 후 Refresh Token을 사용한 갱신 테스트</p>
          </div>
        </div>
      </div>
    </div>

    {/* 더 자세한 가이드 */}
    <div className="bg-gradient-to-r from-indigo-50 to-purple-50 border border-indigo-200 rounded-lg p-6">
      <h3 className="text-lg font-semibold text-gray-900 mb-2">📚 더 자세한 가이드</h3>
      <p className="text-sm text-gray-700 mb-3">
        샘플 서비스의 <code className="bg-white/70 px-1 rounded">samples/README.md</code> 파일에서
        6가지 상세 테스트 시나리오와 트러블슈팅 가이드를 확인할 수 있습니다.
      </p>
      <ul className="space-y-1 text-sm text-gray-600">
        <li>• OAuth 2.0 플로우 다이어그램</li>
        <li>• 서비스 설정 방법</li>
        <li>• 멀티 테넌트 테스트</li>
        <li>• Scope 기반 권한 테스트</li>
        <li>• 트러블슈팅 가이드</li>
      </ul>
    </div>
  </div>
)

interface CodeBlockProps {
  language: string
  code: string
  onCopy: () => void
}

const CodeBlock: React.FC<CodeBlockProps> = ({ language, code, onCopy }) => (
  <div className="relative">
    <div className="absolute top-2 right-2">
      <button
        onClick={onCopy}
        className="p-2 bg-gray-700 hover:bg-gray-600 text-white rounded transition-colors"
        title="복사"
      >
        <ClipboardDocumentIcon className="h-4 w-4" />
      </button>
    </div>
    <pre className="bg-gray-900 text-gray-100 rounded-lg p-4 overflow-x-auto text-sm">
      <code className={`language-${language}`}>{code}</code>
    </pre>
  </div>
)

export default DocsPage
