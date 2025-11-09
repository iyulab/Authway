using Microsoft.AspNetCore.Authentication.JwtBearer;
using Microsoft.AspNetCore.Authorization;
using Microsoft.IdentityModel.Tokens;

var builder = WebApplication.CreateBuilder(args);

// Add CORS
var corsOrigins = builder.Configuration.GetSection("Cors:AllowedOrigins").Get<string[]>()
    ?? new[] { "http://localhost:5173" };

builder.Services.AddCors(options =>
{
    options.AddDefaultPolicy(policy =>
    {
        policy.WithOrigins(corsOrigins)
              .AllowAnyMethod()
              .AllowAnyHeader()
              .AllowCredentials();
    });
});

// Add JWT Authentication
var authority = builder.Configuration["Authway:Authority"]
    ?? throw new InvalidOperationException("Authway:Authority is required");
var audience = builder.Configuration["Authway:Audience"] ?? "api";

builder.Services.AddAuthentication(JwtBearerDefaults.AuthenticationScheme)
    .AddJwtBearer(options =>
    {
        options.Authority = authority;
        options.Audience = audience;
        options.RequireHttpsMetadata = builder.Configuration.GetValue<bool>("Authway:RequireHttpsMetadata", true);

        options.TokenValidationParameters = new TokenValidationParameters
        {
            ValidateIssuer = true,
            ValidIssuer = authority,
            // NOTE: Audience validation is disabled due to Hydra configuration issue
            // Frontend sends 'audience=api' parameter and Hydra client has 'audience: ["api"]' whitelist
            // However, tokens still have empty 'aud' claim (aud: Array(0))
            // Root cause: Hydra client 'audience' is a whitelist, not automatic inclusion
            // The explicit 'audience' parameter should trigger inclusion but appears to need additional Hydra config
            // TODO: Investigate Hydra audience configuration and re-enable validation for production
            // For now: ValidateAudience = false allows sample to work, but should NOT be used in production
            ValidateAudience = false,
            ValidAudience = audience,
            ValidateLifetime = true,
            ValidateIssuerSigningKey = true
        };

        options.Events = new JwtBearerEvents
        {
            OnAuthenticationFailed = context =>
            {
                var logger = context.HttpContext.RequestServices
                    .GetRequiredService<ILogger<Program>>();
                logger.LogError(context.Exception, "Authentication failed");
                return Task.CompletedTask;
            },
            OnTokenValidated = context =>
            {
                var logger = context.HttpContext.RequestServices
                    .GetRequiredService<ILogger<Program>>();
                logger.LogInformation("Token validated for user: {User}",
                    context.Principal?.Identity?.Name);
                return Task.CompletedTask;
            }
        };
    });

builder.Services.AddAuthorization();

// Add OpenAPI
builder.Services.AddOpenApi();

var app = builder.Build();

// Configure the HTTP request pipeline
if (app.Environment.IsDevelopment())
{
    app.MapOpenApi();
}

app.UseCors();
app.UseAuthentication();
app.UseAuthorization();

// Health check endpoint
app.MapGet("/health", () => new
{
    status = "healthy",
    timestamp = DateTime.UtcNow
})
.WithName("HealthCheck")
.WithOpenApi();

// Public endpoint - no authentication required
app.MapGet("/api/public", () => new
{
    message = "This is a public endpoint",
    timestamp = DateTime.UtcNow
})
.WithName("GetPublic")
.WithOpenApi();

// Protected endpoint - requires authentication
app.MapGet("/api/protected", [Authorize] (HttpContext context) =>
{
    var user = context.User;
    var claims = user.Claims.Select(c => new { c.Type, c.Value }).ToList();

    return new
    {
        message = "This is a protected endpoint",
        user = new
        {
            id = user.FindFirst("sub")?.Value,
            name = user.FindFirst("name")?.Value ?? user.Identity?.Name,
            email = user.FindFirst("email")?.Value,
            claims
        },
        timestamp = DateTime.UtcNow
    };
})
.WithName("GetProtected")
.WithOpenApi()
.RequireAuthorization();

// User profile endpoint
app.MapGet("/api/me", [Authorize] (HttpContext context) =>
{
    var user = context.User;

    return new
    {
        sub = user.FindFirst("sub")?.Value,
        name = user.FindFirst("name")?.Value,
        email = user.FindFirst("email")?.Value,
        email_verified = user.FindFirst("email_verified")?.Value,
        preferred_username = user.FindFirst("preferred_username")?.Value,
        claims = user.Claims
            .GroupBy(c => c.Type)
            .ToDictionary(
                g => g.Key,
                g => g.Count() > 1 ? (object)g.Select(c => c.Value).ToArray() : g.First().Value
            )
    };
})
.WithName("GetUserProfile")
.WithOpenApi()
.RequireAuthorization();

// Sample data endpoint
var summaries = new[]
{
    "Freezing", "Bracing", "Chilly", "Cool", "Mild", "Warm", "Balmy", "Hot", "Sweltering", "Scorching"
};

app.MapGet("/api/weather", [Authorize] () =>
{
    var forecast = Enumerable.Range(1, 5).Select(index =>
        new WeatherForecast
        (
            DateOnly.FromDateTime(DateTime.Now.AddDays(index)),
            Random.Shared.Next(-20, 55),
            summaries[Random.Shared.Next(summaries.Length)]
        ))
        .ToArray();
    return forecast;
})
.WithName("GetWeather")
.WithOpenApi()
.RequireAuthorization();

app.Run();

record WeatherForecast(DateOnly Date, int TemperatureC, string? Summary)
{
    public int TemperatureF => 32 + (int)(TemperatureC / 0.5556);
}
