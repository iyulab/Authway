using System.Text.Json.Serialization;

namespace AuthwaySample.Services;

/// <summary>
/// Service to discover Authway configuration from the server
/// </summary>
public class AuthwayConfigService
{
    private readonly HttpClient _httpClient;
    private readonly IConfiguration _configuration;
    private readonly ILogger<AuthwayConfigService> _logger;
    private AuthwayConfig? _cachedConfig;

    public AuthwayConfigService(
        HttpClient httpClient,
        IConfiguration configuration,
        ILogger<AuthwayConfigService> logger)
    {
        _httpClient = httpClient;
        _configuration = configuration;
        _logger = logger;
    }

    /// <summary>
    /// Get Authway configuration from the server
    /// Discovers Hydra URL (OIDC Authority) automatically
    /// </summary>
    public async Task<AuthwayConfig> GetConfigAsync()
    {
        if (_cachedConfig != null)
        {
            return _cachedConfig;
        }

        var serverUrl = _configuration["Authway:Server"];
        if (string.IsNullOrEmpty(serverUrl))
        {
            throw new InvalidOperationException(
                "Authway:Server is required. Add it to your appsettings.json");
        }

        try
        {
            var configUrl = $"{serverUrl.TrimEnd('/')}/.well-known/authway-config";
            _logger.LogInformation("Fetching Authway configuration from {ConfigUrl}", configUrl);

            var response = await _httpClient.GetAsync(configUrl);
            response.EnsureSuccessStatusCode();

            var config = await response.Content.ReadFromJsonAsync<AuthwayConfig>();
            if (config == null)
            {
                throw new InvalidOperationException("Failed to parse Authway configuration");
            }

            _cachedConfig = config;
            _logger.LogInformation("Authway configuration loaded: Issuer={Issuer}", config.Issuer);

            return config;
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "Failed to fetch Authway configuration from {ServerUrl}", serverUrl);
            throw new InvalidOperationException(
                $"Could not connect to Authway server at {serverUrl}. " +
                $"Make sure the server is running and accessible.", ex);
        }
    }

    /// <summary>
    /// Get the OIDC Authority URL (Hydra Public URL)
    /// </summary>
    public async Task<string> GetAuthorityAsync()
    {
        var config = await GetConfigAsync();
        return config.Issuer;
    }

    /// <summary>
    /// Get the API Server URL
    /// </summary>
    public async Task<string> GetApiServerAsync()
    {
        var config = await GetConfigAsync();
        return config.ApiServer;
    }
}

public class AuthwayConfig
{
    [JsonPropertyName("issuer")]
    public string Issuer { get; set; } = string.Empty;

    [JsonPropertyName("oauth_url")]
    public string OAuthUrl { get; set; } = string.Empty;

    [JsonPropertyName("api_url")]
    public string ApiUrl { get; set; } = string.Empty;

    [JsonPropertyName("version")]
    public string Version { get; set; } = string.Empty;

    // Alias for compatibility
    public string AuthServer => ApiUrl;
    public string ApiServer => ApiUrl;
}
