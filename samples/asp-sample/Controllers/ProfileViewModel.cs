namespace AuthwaySample.Controllers;

public class ProfileViewModel
{
    public string? UserName { get; set; }
    public bool IsAuthenticated { get; set; }
    public string? AuthenticationType { get; set; }
    public Dictionary<string, string> Claims { get; set; } = new();
    public string? AccessToken { get; set; }
    public string? IdToken { get; set; }
    public string? RefreshToken { get; set; }
}
