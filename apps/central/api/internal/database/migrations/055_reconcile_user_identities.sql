-- ============================================================
-- Migration 055: Reading sign-in links from user_identities
-- ============================================================
-- Until now an account's links to sign-in providers were read from the four
-- provider columns on users and written to the user_identities rows beside
-- them (migration 054). From this release the rows are what is read, and an
-- account may have several at one provider.
--
-- Before it is, the rows are made to match the columns once more: an API that
-- ran from before 054 wrote the columns alone, so a link it made has no row
-- and a link it removed still has one. After this, every API that serves
-- writes the rows and keeps the columns at the oldest identity of each
-- provider, so an API still running from the release before reads the same
-- links for as long as an account has one identity per provider.
--
-- Run once, while no account yet has two identities at a provider: it makes
-- the rows equal to the columns, and would remove a second identity.
--
-- NOTE: No BEGIN/COMMIT here. RunMigrations wraps the whole run in a single
-- outer transaction.
-- ============================================================

DELETE FROM user_identities i
WHERE NOT EXISTS (
    SELECT 1 FROM users u
    WHERE u.id = i.user_id
      AND ((i.provider = 'google' AND u.google_id = i.subject)
        OR (i.provider = 'github' AND u.github_id = i.subject)
        OR (i.provider = 'microsoft' AND u.microsoft_id = i.subject)
        OR (i.provider = 'apple' AND u.apple_id = i.subject))
);

INSERT INTO user_identities (id, tenant_id, user_id, provider, subject, email_at_link, linked_at)
SELECT gen_random_uuid(), u.tenant_id, u.id, l.provider, l.subject, u.email, u.updated_at
FROM users u
CROSS JOIN LATERAL (VALUES
    ('google', u.google_id),
    ('github', u.github_id),
    ('microsoft', u.microsoft_id),
    ('apple', u.apple_id)
) AS l(provider, subject)
WHERE l.subject IS NOT NULL AND l.subject <> ''
  AND NOT EXISTS (SELECT 1 FROM user_identities i WHERE i.user_id = u.id AND i.provider = l.provider)
ON CONFLICT DO NOTHING;
