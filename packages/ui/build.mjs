// Builds the two bundled screens for a single-origin installation — the login
// UI for the site root, the console for /console/, both reaching the API on
// their own origin — into login/ and console/ beside this file. The
// on-premises web image and this package ship the same output.
//
//   node packages/ui/build.mjs [--no-install]
//
// --no-install skips `npm ci` in each app (a Docker build installs them in an
// earlier, cached layer).
import { execSync } from 'node:child_process'
import { cpSync, rmSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const root = resolve(here, '../..')
const install = !process.argv.includes('--no-install')

const screens = [
  { app: 'apps/branding/auth-ui', out: 'login', env: {} },
  { app: 'apps/central/admin', out: 'console', env: { ADMIN_BASE_PATH: '/console/' } },
]

// Hosting files for the separate-origin deployments; nothing in an
// installation that serves these folders reads them.
const hostingFiles = ['_headers', '_redirects', 'staticwebapp.config.json']

for (const { app, out, env } of screens) {
  const cwd = join(root, app)
  // An empty API URL — the screen's own origin — whatever a .env file in the
  // app would say (src/config.ts).
  const buildEnv = { ...process.env, VITE_API_URL: '', ...env }
  if (install) execSync('npm ci --no-audit --no-fund', { cwd, stdio: 'inherit' })
  execSync('npm run build', { cwd, stdio: 'inherit', env: buildEnv })

  const target = join(here, out)
  rmSync(target, { recursive: true, force: true })
  cpSync(join(cwd, 'dist'), target, { recursive: true })
  for (const file of hostingFiles) rmSync(join(target, file), { force: true })
}
