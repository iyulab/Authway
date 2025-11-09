using System.Net.Http.Headers;
using System.Text;
using System.Text.Json;
using System.Text.Json.Serialization;

namespace AuthwaySample.Services;

public class ClaimsService
{
    private readonly HttpClient _httpClient;
    private readonly IConfiguration _configuration;
    private readonly ILogger<ClaimsService> _logger;

    public ClaimsService(HttpClient httpClient, IConfiguration configuration, ILogger<ClaimsService> logger)
    {
        _httpClient = httpClient;
        _configuration = configuration;
        _logger = logger;
    }

    public async Task<UpdateClaimsResponse> UpdateClaimsAsync(
        string accessToken,
        Dictionary<string, object> claims,
        bool permanent = false)
    {
        // Get API Server URL - Server is the primary config
        var authwayUrl = _configuration["Authway:Server"]
                         ?? _configuration["Authway:ApiUrl"]  // Backward compatibility
                         ?? _configuration["Authway:Api"];    // Backward compatibility

        if (string.IsNullOrEmpty(authwayUrl))
        {
            throw new InvalidOperationException(
                "Authway:Server must be configured to use Dynamic Claims feature. " +
                "Add \"Server\": \"https://authway-api.iyulab.com\" to your appsettings.json");
        }

        // Use the new endpoint that doesn't require re-authentication
        var request = new UpdateUserClaimsRequest
        {
            Claims = claims
        };

        var json = JsonSerializer.Serialize(request, new JsonSerializerOptions
        {
            PropertyNamingPolicy = JsonNamingPolicy.SnakeCaseLower
        });
        var content = new StringContent(json, Encoding.UTF8, "application/json");

        var httpRequest = new HttpRequestMessage(HttpMethod.Patch, $"{authwayUrl}/api/v1/claims/user")
        {
            Content = content
        };
        httpRequest.Headers.Authorization = new AuthenticationHeaderValue("Bearer", accessToken);

        try
        {
            var response = await _httpClient.SendAsync(httpRequest);
            response.EnsureSuccessStatusCode();

            var responseBody = await response.Content.ReadAsStringAsync();
            var userClaimsResponse = JsonSerializer.Deserialize<UpdateUserClaimsResponse>(responseBody, new JsonSerializerOptions
            {
                PropertyNamingPolicy = JsonNamingPolicy.SnakeCaseLower
            }) ?? throw new Exception("Failed to deserialize response");

            // Convert to UpdateClaimsResponse format
            return new UpdateClaimsResponse
            {
                Success = userClaimsResponse.Success,
                Message = userClaimsResponse.Message,
                AuthUrl = "" // No re-auth needed
            };
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "Failed to update claims");
            throw;
        }
    }

    public async Task<GetClaimsResponse> GetClaimsAsync(string accessToken)
    {
        var authwayUrl = _configuration["Authway:Server"]
                         ?? _configuration["Authway:ApiUrl"]  // Backward compatibility
                         ?? _configuration["Authway:Api"];    // Backward compatibility

        if (string.IsNullOrEmpty(authwayUrl))
        {
            throw new InvalidOperationException(
                "Authway:Server must be configured to use Dynamic Claims feature. " +
                "Add \"Server\": \"https://authway-api.iyulab.com\" to your appsettings.json");
        }

        var httpRequest = new HttpRequestMessage(HttpMethod.Get, $"{authwayUrl}/api/v1/claims");
        httpRequest.Headers.Authorization = new AuthenticationHeaderValue("Bearer", accessToken);

        try
        {
            var response = await _httpClient.SendAsync(httpRequest);
            response.EnsureSuccessStatusCode();

            var responseBody = await response.Content.ReadAsStringAsync();
            return JsonSerializer.Deserialize<GetClaimsResponse>(responseBody, new JsonSerializerOptions
            {
                PropertyNamingPolicy = JsonNamingPolicy.SnakeCaseLower
            }) ?? throw new Exception("Failed to deserialize response");
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "Failed to get claims");
            throw;
        }
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

public class UpdateUserClaimsRequest
{
    [JsonPropertyName("claims")]
    public Dictionary<string, object> Claims { get; set; } = new();
}

public class UpdateUserClaimsResponse
{
    [JsonPropertyName("success")]
    public bool Success { get; set; }

    [JsonPropertyName("message")]
    public string Message { get; set; } = string.Empty;
}

public class UpdateClaimsResponse
{
    [JsonPropertyName("success")]
    public bool Success { get; set; }

    [JsonPropertyName("auth_url")]
    public string AuthUrl { get; set; } = string.Empty;

    [JsonPropertyName("message")]
    public string Message { get; set; } = string.Empty;
}

public class GetClaimsResponse
{
    [JsonPropertyName("user_id")]
    public string UserId { get; set; } = string.Empty;

    [JsonPropertyName("tenant_id")]
    public string TenantId { get; set; } = string.Empty;

    [JsonPropertyName("claims")]
    public Dictionary<string, object> Claims { get; set; } = new();

    [JsonPropertyName("permanent_claims")]
    public List<string> PermanentClaims { get; set; } = new();

    [JsonPropertyName("session_claims")]
    public List<string> SessionClaims { get; set; } = new();
}
