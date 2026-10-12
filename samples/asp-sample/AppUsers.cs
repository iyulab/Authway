using System.Collections.Concurrent;

namespace AuthwaySample;

/// <summary>
/// The application's own record of a person: who they are here and what they
/// may do here. The identity provider says who signed in; it does not decide
/// this.
/// </summary>
public sealed record AppUser(Guid Id, string DisplayName, string Email, string Role);

/// <summary>
/// The application's users, keyed by the provider's stable identifier for
/// them: the issuer together with the <c>sub</c> claim. Kept in memory so the
/// sample runs without a database; a real application stores this table.
/// </summary>
public sealed class AppUsers
{
    private readonly ConcurrentDictionary<(string Issuer, string Subject), AppUser> _users = new();

    /// <summary>
    /// Finds the user the provider signed in, or creates their record on a
    /// first visit. <paramref name="email"/> and <paramref name="name"/> seed a
    /// new record and are not read again: the application owns them from then
    /// on, and never looks a user up by email.
    /// </summary>
    public AppUser FindOrCreate(string issuer, string subject, string? email, string? name) =>
        _users.GetOrAdd((issuer, subject), _ => new AppUser(
            Guid.NewGuid(),
            string.IsNullOrWhiteSpace(name) ? email ?? subject : name,
            email ?? "",
            // Roles are the application's to give; a new user starts as a member.
            Role: "member"));
}
