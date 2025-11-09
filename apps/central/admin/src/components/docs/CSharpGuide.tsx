import React from 'react'
import { ClipboardDocumentIcon } from '@heroicons/react/24/outline'

interface CodeBlockProps {
  language: string
  code: string
  onCopy: () => void
}

const CodeBlock: React.FC<CodeBlockProps> = ({ language, code, onCopy }) => (
  <div className="relative">
    <div className="absolute top-2 right-2 z-10">
      <button
        onClick={onCopy}
        className="p-2 bg-gray-700 hover:bg-gray-600 text-white rounded transition-colors"
        title="복사"
      >
        <ClipboardDocumentIcon className="h-4 w-4" />
      </button>
    </div>
    <pre className="bg-gray-900 text-gray-100 p-4 rounded-lg overflow-x-auto">
      <code className={`language-${language}`}>{code}</code>
    </pre>
  </div>
)

const platformLogo = (platform: string) => {
  const logos: { [key: string]: { url: string; alt: string } } = {
    csharp: {
      url: 'https://upload.wikimedia.org/wikipedia/commons/e/ee/.NET_Core_Logo.svg',
      alt: '.NET',
    },
  }
  return logos[platform] || null
}

interface CSharpGuideProps {
  copyToClipboard: (text: string, label: string) => void
}

export const CSharpGuide: React.FC<CSharpGuideProps> = ({ copyToClipboard }) => {
  const logo = platformLogo('csharp')

  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-2xl font-bold text-gray-900 flex items-center">
          {logo && <img src={logo.url} alt={logo.alt} className="h-8 w-8 mr-3" />}
          C# ASP.NET에서 Authway 사용하기
        </h2>
        <p className="mt-2 text-gray-600">Microsoft.AspNetCore.Authentication.OpenIdConnect를 사용한 통합</p>
      </div>

      {/* Step 1: 패키지 설치 */}
      <div className="border-l-4 border-purple-600 pl-6">
        <div className="flex items-center space-x-3 mb-3">
          <span className="flex items-center justify-center w-8 h-8 rounded-full bg-purple-600 text-white font-bold">1</span>
          <h3 className="text-xl font-semibold text-gray-900">NuGet 패키지 설치</h3>
        </div>
        <CodeBlock
          language="bash"
          code={`dotnet add package Microsoft.AspNetCore.Authentication.OpenIdConnect
dotnet add package Microsoft.AspNetCore.Authentication.Cookies
dotnet add package System.IdentityModel.Tokens.Jwt`}
          onCopy={() => copyToClipboard('dotnet add package Microsoft.AspNetCore.Authentication.OpenIdConnect\ndotnet add package Microsoft.AspNetCore.Authentication.Cookies\ndotnet add package System.IdentityModel.Tokens.Jwt', '명령어')}
        />
      </div>

      {/* Step 2: appsettings.json */}
      <div className="border-l-4 border-purple-600 pl-6">
        <div className="flex items-center space-x-3 mb-3">
          <span className="flex items-center justify-center w-8 h-8 rounded-full bg-purple-600 text-white font-bold">2</span>
          <h3 className="text-xl font-semibold text-gray-900">appsettings.json 설정</h3>
        </div>
        <CodeBlock
          language="json"
          code={`{
  "Authway": {
    "Authway": "http://localhost:4444",
    "Api": "http://localhost:8080",
    "ClientId": "your-client-id",
    "ClientSecret": "your-client-secret",
    "RedirectUri": "https://localhost:5001/signin-oidc"
  }
}`}
          onCopy={() => copyToClipboard(`{
  "Authway": {
    "Authway": "http://localhost:4444",
    "Api": "http://localhost:8080",
    "ClientId": "your-client-id",
    "ClientSecret": "your-client-secret",
    "RedirectUri": "https://localhost:5001/signin-oidc"
  }
}`, 'JSON 설정')}
        />
      </div>

      {/* Step 3: Program.cs - 핵심 업데이트 */}
      <div className="border-l-4 border-purple-600 pl-6">
        <div className="flex items-center space-x-3 mb-3">
          <span className="flex items-center justify-center w-8 h-8 rounded-full bg-purple-600 text-white font-bold">3</span>
          <h3 className="text-xl font-semibold text-gray-900">Program.cs 설정</h3>
        </div>
        <div className="mb-4 bg-yellow-50 border-l-4 border-yellow-400 p-4">
          <p className="text-sm text-yellow-700">
            <strong>⚠️ 중요:</strong> OnRedirectToIdentityProvider에서 Prompt와 MaxAge를 null로 설정해야 토큰 업데이트 시 consent_required 에러를 방지할 수 있습니다.
          </p>
        </div>
        <CodeBlock
          language="csharp"
          code={`using Microsoft.AspNetCore.Authentication.Cookies;
using Microsoft.AspNetCore.Authentication.OpenIdConnect;
using Microsoft.IdentityModel.Protocols.OpenIdConnect;

var builder = WebApplication.CreateBuilder(args);

builder.Services.AddControllersWithViews();

// Configure Authentication
builder.Services.AddAuthentication(options =>
{
    options.DefaultScheme = CookieAuthenticationDefaults.AuthenticationScheme;
    options.DefaultChallengeScheme = OpenIdConnectDefaults.AuthenticationScheme;
})
.AddCookie(options =>
{
    options.Cookie.Name = "authway-app";
    options.ExpireTimeSpan = TimeSpan.FromHours(1);
    options.SlidingExpiration = true;
})
.AddOpenIdConnect(options =>
{
    options.Authority = builder.Configuration["Authway:Authway"];
    options.ClientId = builder.Configuration["Authway:ClientId"];
    options.ClientSecret = builder.Configuration["Authway:ClientSecret"];

    options.ResponseType = OpenIdConnectResponseType.Code;
    options.UsePkce = true;
    options.SaveTokens = true;
    options.GetClaimsFromUserInfoEndpoint = true;

    options.Scope.Clear();
    options.Scope.Add("openid");
    options.Scope.Add("profile");
    options.Scope.Add("email");

    options.CallbackPath = "/signin-oidc";
    options.SignedOutCallbackPath = "/signout-callback-oidc";

    options.RequireHttpsMetadata = !builder.Environment.IsDevelopment();
    options.ProtocolValidator.RequireNonce = false;

    // 🔑 Critical for token updates and workspace switching
    options.Events = new OpenIdConnectEvents
    {
        OnRedirectToIdentityProvider = context =>
        {
            // Remove prompt parameter to avoid consent_required errors
            // Essential for workspace switching and token updates
            context.ProtocolMessage.Prompt = null;
            context.ProtocolMessage.MaxAge = null;
            return Task.CompletedTask;
        },
        OnRemoteFailure = context =>
        {
            var logger = context.HttpContext.RequestServices
                .GetRequiredService<ILogger<Program>>();

            // Handle consent_required errors gracefully
            if (context.Failure?.Message?.Contains("consent_required") == true)
            {
                logger.LogWarning("Consent required, redirecting to login");
                context.Response.Redirect("/Home/Login?returnUrl=/Home/Profile");
                context.HandleResponse();
                return Task.CompletedTask;
            }

            logger.LogError(context.Failure, "Authentication failed");
            context.Response.Redirect("/Home/Error");
            context.HandleResponse();
            return Task.CompletedTask;
        }
    };
});

var app = builder.Build();

if (!app.Environment.IsDevelopment())
{
    app.UseExceptionHandler("/Home/Error");
    app.UseHsts();
}

app.UseHttpsRedirection();
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
using Microsoft.IdentityModel.Protocols.OpenIdConnect;

var builder = WebApplication.CreateBuilder(args);
builder.Services.AddControllersWithViews();

builder.Services.AddAuthentication(options =>
{
    options.DefaultScheme = CookieAuthenticationDefaults.AuthenticationScheme;
    options.DefaultChallengeScheme = OpenIdConnectDefaults.AuthenticationScheme;
})
.AddCookie(options =>
{
    options.Cookie.Name = "authway-app";
    options.ExpireTimeSpan = TimeSpan.FromHours(1);
    options.SlidingExpiration = true;
})
.AddOpenIdConnect(options =>
{
    options.Authority = builder.Configuration["Authway:Authway"];
    options.ClientId = builder.Configuration["Authway:ClientId"];
    options.ClientSecret = builder.Configuration["Authway:ClientSecret"];

    options.ResponseType = OpenIdConnectResponseType.Code;
    options.UsePkce = true;
    options.SaveTokens = true;
    options.GetClaimsFromUserInfoEndpoint = true;

    options.Scope.Clear();
    options.Scope.Add("openid");
    options.Scope.Add("profile");
    options.Scope.Add("email");

    options.CallbackPath = "/signin-oidc";
    options.SignedOutCallbackPath = "/signout-callback-oidc";

    options.RequireHttpsMetadata = !builder.Environment.IsDevelopment();
    options.ProtocolValidator.RequireNonce = false;

    options.Events = new OpenIdConnectEvents
    {
        OnRedirectToIdentityProvider = context =>
        {
            context.ProtocolMessage.Prompt = null;
            context.ProtocolMessage.MaxAge = null;
            return Task.CompletedTask;
        },
        OnRemoteFailure = context =>
        {
            var logger = context.HttpContext.RequestServices.GetRequiredService<ILogger<Program>>();
            if (context.Failure?.Message?.Contains("consent_required") == true)
            {
                logger.LogWarning("Consent required, redirecting to login");
                context.Response.Redirect("/Home/Login?returnUrl=/Home/Profile");
                context.HandleResponse();
                return Task.CompletedTask;
            }
            logger.LogError(context.Failure, "Authentication failed");
            context.Response.Redirect("/Home/Error");
            context.HandleResponse();
            return Task.CompletedTask;
        }
    };
});`, 'C# 코드')}
        />
      </div>

      {/* Step 4: ClaimsService for Workspace Switching */}
      <div className="border-l-4 border-purple-600 pl-6">
        <div className="flex items-center space-x-3 mb-3">
          <span className="flex items-center justify-center w-8 h-8 rounded-full bg-purple-600 text-white font-bold">4</span>
          <h3 className="text-xl font-semibold text-gray-900">Workspace 전환을 위한 ClaimsService</h3>
        </div>
        <div className="mb-4 bg-blue-50 border-l-4 border-blue-400 p-4">
          <p className="text-sm text-blue-700">
            <strong>💡 Tip:</strong> ClaimsService를 사용하면 사용자가 workspace를 전환할 때 토큰을 자동으로 업데이트할 수 있습니다.
          </p>
        </div>
        <CodeBlock
          language="csharp"
          code={`// Services/ClaimsService.cs
using System.Net.Http.Headers;
using System.Text;
using System.Text.Json;
using System.Text.Json.Serialization;

public class ClaimsService
{
    private readonly HttpClient _httpClient;
    private readonly IConfiguration _configuration;

    public ClaimsService(HttpClient httpClient, IConfiguration configuration)
    {
        _httpClient = httpClient;
        _configuration = configuration;
    }

    public async Task<UpdateClaimsResponse> UpdateClaimsAsync(
        string accessToken,
        Dictionary<string, object> claims,
        bool permanent = false)
    {
        var authwayUrl = _configuration["Authway:Api"];
        var clientId = _configuration["Authway:ClientId"];
        var redirectUri = _configuration["Authway:RedirectUri"];

        var request = new UpdateClaimsRequest
        {
            Claims = claims,
            Permanent = permanent,
            ClientId = clientId,
            RedirectUri = redirectUri
        };

        var json = JsonSerializer.Serialize(request, new JsonSerializerOptions
        {
            PropertyNamingPolicy = JsonNamingPolicy.SnakeCaseLower
        });
        var content = new StringContent(json, Encoding.UTF8, "application/json");

        var httpRequest = new HttpRequestMessage(HttpMethod.Post,
            $"{authwayUrl}/api/v1/claims/update")
        {
            Content = content
        };
        httpRequest.Headers.Authorization =
            new AuthenticationHeaderValue("Bearer", accessToken);

        var response = await _httpClient.SendAsync(httpRequest);
        response.EnsureSuccessStatusCode();

        var responseBody = await response.Content.ReadAsStringAsync();
        return JsonSerializer.Deserialize<UpdateClaimsResponse>(responseBody,
            new JsonSerializerOptions
            {
                PropertyNamingPolicy = JsonNamingPolicy.SnakeCaseLower
            });
    }
}

public class UpdateClaimsRequest
{
    [JsonPropertyName("claims")]
    public Dictionary<string, object> Claims { get; set; } = new();

    [JsonPropertyName("permanent")]
    public bool Permanent { get; set; }

    [JsonPropertyName("client_id")]
    public string ClientId { get; set; } = string.Empty;

    [JsonPropertyName("redirect_uri")]
    public string RedirectUri { get; set; } = string.Empty;
}

public class UpdateClaimsResponse
{
    [JsonPropertyName("success")]
    public bool Success { get; set; }

    [JsonPropertyName("auth_url")]
    public string AuthUrl { get; set; } = string.Empty;

    [JsonPropertyName("message")]
    public string Message { get; set; } = string.Empty;
}`}
          onCopy={() => copyToClipboard(`// Services/ClaimsService.cs
public class ClaimsService
{
    public async Task<UpdateClaimsResponse> UpdateClaimsAsync(
        string accessToken,
        Dictionary<string, object> claims,
        bool permanent = false)
    {
        // Implementation...
    }
}`, 'C# 코드')}
        />
      </div>

      {/* Step 5: WorkspaceController */}
      <div className="border-l-4 border-purple-600 pl-6">
        <div className="flex items-center space-x-3 mb-3">
          <span className="flex items-center justify-center w-8 h-8 rounded-full bg-purple-600 text-white font-bold">5</span>
          <h3 className="text-xl font-semibold text-gray-900">Workspace 전환 Controller</h3>
        </div>
        <CodeBlock
          language="csharp"
          code={`// Controllers/WorkspaceController.cs
using Microsoft.AspNetCore.Authentication;
using Microsoft.AspNetCore.Authorization;
using Microsoft.AspNetCore.Mvc;

[Authorize]
public class WorkspaceController : Controller
{
    private readonly ClaimsService _claimsService;

    public WorkspaceController(ClaimsService claimsService)
    {
        _claimsService = claimsService;
    }

    [HttpPost("switch")]
    public async Task<IActionResult> SwitchWorkspace(
        [FromBody] SwitchWorkspaceRequest request)
    {
        // Get access token
        var accessToken = await HttpContext.GetTokenAsync("access_token");
        if (string.IsNullOrEmpty(accessToken))
        {
            return Unauthorized(new { error = "No access token found" });
        }

        try
        {
            // Update claims with new workspace
            var result = await _claimsService.UpdateClaimsAsync(
                accessToken,
                new Dictionary<string, object>
                {
                    ["workspace_id"] = request.WorkspaceId,
                    ["workspace_name"] = request.WorkspaceName
                }
            );

            // Return auth URL for re-authentication
            return Ok(new { authUrl = result.AuthUrl });
        }
        catch (Exception ex)
        {
            return StatusCode(500, new { error = ex.Message });
        }
    }
}

public record SwitchWorkspaceRequest(string WorkspaceId, string WorkspaceName);`}
          onCopy={() => copyToClipboard(`[Authorize]
public class WorkspaceController : Controller
{
    private readonly ClaimsService _claimsService;

    public WorkspaceController(ClaimsService claimsService)
    {
        _claimsService = claimsService;
    }

    [HttpPost("switch")]
    public async Task<IActionResult> SwitchWorkspace([FromBody] SwitchWorkspaceRequest request)
    {
        var accessToken = await HttpContext.GetTokenAsync("access_token");
        var result = await _claimsService.UpdateClaimsAsync(
            accessToken,
            new Dictionary<string, object>
            {
                ["workspace_id"] = request.WorkspaceId,
                ["workspace_name"] = request.WorkspaceName
            }
        );
        return Ok(new { authUrl = result.AuthUrl });
    }
}`, 'C# 코드')}
        />
      </div>

      {/* Step 6: Frontend Integration */}
      <div className="border-l-4 border-purple-600 pl-6">
        <div className="flex items-center space-x-3 mb-3">
          <span className="flex items-center justify-center w-8 h-8 rounded-full bg-purple-600 text-white font-bold">6</span>
          <h3 className="text-xl font-semibold text-gray-900">프론트엔드 통합</h3>
        </div>
        <CodeBlock
          language="javascript"
          code={`// Workspace 전환 JavaScript
async function switchWorkspace(workspaceId, workspaceName) {
    try {
        const response = await fetch('/workspace/switch', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({
                workspaceId: workspaceId,
                workspaceName: workspaceName
            })
        });

        const data = await response.json();

        if (data.authUrl) {
            // Redirect to re-authenticate with new claims
            window.location.href = data.authUrl;
        }
    } catch (error) {
        console.error('Failed to switch workspace:', error);
    }
}`}
          onCopy={() => copyToClipboard(`async function switchWorkspace(workspaceId, workspaceName) {
    const response = await fetch('/workspace/switch', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ workspaceId, workspaceName })
    });
    const data = await response.json();
    if (data.authUrl) window.location.href = data.authUrl;
}`, 'JavaScript 코드')}
        />
      </div>

      {/* 완료 메시지 */}
      <div className="bg-green-50 border border-green-200 rounded-lg p-6">
        <h3 className="text-lg font-semibold text-green-900 mb-2">✅ 통합 완료!</h3>
        <p className="text-green-700">
          이제 ASP.NET Core 애플리케이션이 Authway와 통합되었습니다.
        </p>
        <ul className="mt-4 space-y-2 text-green-700">
          <li className="flex items-start">
            <span className="mr-2">✓</span>
            <span>OAuth 2.0 / OpenID Connect 인증</span>
          </li>
          <li className="flex items-start">
            <span className="mr-2">✓</span>
            <span>토큰 기반 API 호출</span>
          </li>
          <li className="flex items-start">
            <span className="mr-2">✓</span>
            <span>Workspace 전환 시 자동 토큰 업데이트</span>
          </li>
          <li className="flex items-start">
            <span className="mr-2">✓</span>
            <span>Consent required 에러 방지</span>
          </li>
        </ul>
      </div>

      {/* 추가 리소스 */}
      <div className="bg-indigo-50 border border-indigo-200 rounded-lg p-6">
        <h3 className="text-lg font-semibold text-indigo-900 mb-3">📚 추가 리소스</h3>
        <ul className="space-y-2 text-indigo-700">
          <li>
            <a href="https://github.com/iyulab/authway/tree/main/samples/asp-sample"
               className="hover:underline" target="_blank" rel="noopener noreferrer">
              → 완전한 ASP.NET 샘플 코드 보기
            </a>
          </li>
          <li>
            <a href="https://docs.microsoft.com/aspnet/core/security/authentication/"
               className="hover:underline" target="_blank" rel="noopener noreferrer">
              → ASP.NET Core 인증 공식 문서
            </a>
          </li>
        </ul>
      </div>
    </div>
  )
}
