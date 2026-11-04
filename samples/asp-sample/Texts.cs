using System.Globalization;

namespace AuthwaySample;

// The words this sample puts on screen, in the two languages Authway
// supports. The language is the request's UI culture (see UseRequestLocalization
// in Program.cs): ?lang=, then the choice remembered in a cookie, then the
// browser's Accept-Language, otherwise English. An application with more text
// would use resource files (IStringLocalizer); the table keeps the sample small.
public static class Texts
{
    public static readonly string[] Languages = ["en", "ko"];

    static readonly Dictionary<string, (string En, string Ko)> Table = new()
    {
        ["site"] = ("Authway ASP.NET sample", "Authway ASP.NET 샘플"),
        ["signIn"] = ("Sign in", "로그인"),
        ["signOut"] = ("Sign out", "로그아웃"),

        ["home.title"] = ("Home", "홈"),
        ["home.heading"] = ("A server-side relying party", "서버 측 Relying Party"),
        ["home.signedInAs"] = ("Signed in as", "로그인한 사용자:"),
        ["home.yourAccount"] = ("Your account", "내 계정"),
        ["home.explain"] = (
            "Signing in sends you to the identity provider and brings you back with a session that belongs to this application.",
            "로그인하면 ID 공급자로 이동했다가, 이 애플리케이션 고유의 세션과 함께 돌아옵니다."),

        ["account.title"] = ("Account", "계정"),
        ["account.heading"] = ("Your account", "내 계정"),
        ["account.intro"] = (
            "This is the application's own record of you. The identity provider told the application who signed in; what you may do here is the application's decision.",
            "이 애플리케이션이 가진 사용자 기록입니다. ID 공급자는 누가 로그인했는지만 알려 주며, 여기서 무엇을 할 수 있는지는 애플리케이션이 정합니다."),
        ["account.userId"] = ("User id (this application's)", "사용자 ID (이 애플리케이션의)"),
        ["account.name"] = ("Name", "이름"),
        ["account.role"] = ("Role (given by this application)", "역할 (이 애플리케이션이 부여)"),

        ["error.title"] = ("Error", "오류"),
        ["error.heading"] = ("Something went wrong", "문제가 발생했습니다"),
        ["error.back"] = ("Back to the start", "처음으로"),
    };

    /// <summary>The language of the current request: "en" or "ko".</summary>
    public static string Language =>
        CultureInfo.CurrentUICulture.TwoLetterISOLanguageName == "ko" ? "ko" : "en";

    /// <summary>The text for <paramref name="key"/> in the current request's language.</summary>
    public static string Get(string key) =>
        Table.TryGetValue(key, out var text) ? (Language == "ko" ? text.Ko : text.En) : key;

    /// <summary>The language the page can be switched to, and its name in that language.</summary>
    public static (string Code, string Name) Other =>
        Language == "ko" ? ("en", "English") : ("ko", "한국어");
}
