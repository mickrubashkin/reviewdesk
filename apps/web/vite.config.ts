import { execSync } from 'node:child_process'
import { readFileSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { defineConfig, loadEnv, searchForWorkspaceRoot, type Plugin } from 'vite'
import react from '@vitejs/plugin-react'

function git(args: string) {
  try {
    return execSync(`git ${args}`, { stdio: ['ignore', 'pipe', 'ignore'] }).toString().trim()
  } catch {
    return ''
  }
}

const { version } = JSON.parse(readFileSync('./package.json', 'utf-8')) as { version: string }

const demoUrl = 'https://demo.reviewdesk.rubashkin.xyz'

// demoSharePreview lets link previews (LinkedIn) show the public demo: the
// demo build gets Open Graph tags and a robots.txt that lets LinkedInBot in.
// Other builds stay closed to every bot and carry no demo text.
function demoSharePreview(enabled: boolean): Plugin {
  let outDir = 'dist'
  return {
    name: 'demo-share-preview',
    apply: 'build',
    configResolved(config) {
      outDir = config.build.outDir
    },
    transformIndexHtml(html) {
      if (!enabled) {
        return html
      }
      const tags = [
        ['og:title', 'ReviewDesk — live demo'],
        ['og:description', 'Review and manage multilingual onboarding email chains. Built with Go, React and PostgreSQL. Sample data, no sign-up.'],
        // Bump ?v= when the image changes: LinkedIn caches images by URL.
        ['og:image', `${demoUrl}/og-image.png?v=2`],
        ['og:image:width', '1200'],
        ['og:image:height', '627'],
        ['og:url', `${demoUrl}/`],
        ['og:type', 'website'],
      ]
        .map(([property, content]) => `  <meta property="${property}" content="${content}" />`)
        .join('\n')
      return html.replace('</head>', `${tags}\n</head>`)
    },
    // After Vite copies public/, whose robots.txt closes the site to all bots.
    closeBundle() {
      if (enabled) {
        writeFileSync(join(outDir, 'robots.txt'), 'User-agent: LinkedInBot\nAllow: /\n\nUser-agent: *\nDisallow: /\n')
      }
    },
  }
}

// https://vite.dev/config/
export default defineConfig(({ mode }) => ({
  envDir: "../..",
  plugins: [react(), demoSharePreview(loadEnv(mode, '../..', 'VITE_').VITE_AUTH_DEMO_LOGIN_ENABLED === 'true')],
  define: {
    __APP_VERSION__: JSON.stringify(version),
    __APP_COMMIT__: JSON.stringify(git('rev-parse --short HEAD')),
    // Date of the last commit, i.e. the last update; falls back to build time.
    __APP_UPDATED_AT__: JSON.stringify(git('log -1 --format=%cI') || new Date().toISOString()),
  },
  server: {
    // The help pages are Markdown in docs/help, outside this app.
    fs: {
      allow: [searchForWorkspaceRoot(process.cwd()), '../../docs/help'],
    },
    proxy: {
      '/api': 'http://localhost:8080',
      '/health': 'http://localhost:8080',
    }
  }
}))
