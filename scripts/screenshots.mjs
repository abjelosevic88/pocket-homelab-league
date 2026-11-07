// README screenshots and a browser smoke test, against a demo instance (DEMO_DATA=true seeds three groups and four months of games):
//
//   DATA_DIR=/tmp/league-demo DEMO_DATA=true ADMIN_EMAIL=demo@example.com ADMIN_PASSWORD=demodemo1 ADMIN_NAME=Aleksandar PORT=18250 ./pocket-league
//   CDP=http://127.0.0.1:9340 BASE=http://127.0.0.1:18250 node scripts/screenshots.mjs   # any Chrome with remote debugging
//   SHOTS=0 … node scripts/screenshots.mjs                                                # checks only
// playwright-core comes from web/node_modules, or set PW_FROM to a package.json next to an install of it.
import { createRequire } from 'node:module'
import assert from 'node:assert/strict'

const require = createRequire(process.env.PW_FROM || new URL('../web/package.json', import.meta.url))
let pw
try { pw = require('playwright') } catch { pw = require('playwright-core') }
const { chromium } = pw
const base = process.env.BASE || 'http://127.0.0.1:18250'
const shots = process.env.SHOTS !== '0'
const only = process.env.ONLY || ''
const dir = process.env.OUT || new URL('../docs/screenshots/', import.meta.url).pathname
const browser = process.env.CDP ? await chromium.connectOverCDP(process.env.CDP) : await chromium.launch()
const errors = []

async function session({ phone = false, dark = false } = {}) {
  const ctx = await browser.newContext({
    viewport: phone ? { width: 390, height: 844 } : { width: 1440, height: 900 },
    deviceScaleFactor: phone ? 3 : 2, isMobile: phone, hasTouch: phone, colorScheme: dark ? 'dark' : 'light',
    userAgent: phone ? 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1' : undefined,
  })
  const page = await ctx.newPage()
  page.on('pageerror', e => errors.push(`pageerror: ${e}`))
  page.on('console', m => { if (m.type() === 'error' && !/Failed to load resource|Content Security Policy/.test(m.text())) errors.push(m.text()) })
  await page.goto(base + '/login')
  const res = await page.evaluate(() => fetch('/api/v1/auth/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email: 'demo@example.com', password: 'demodemo1' }) }).then(r => r.status))
  assert.equal(res, 200, 'demo sign-in')
  return { ctx, page }
}
async function go(page, path, wait = 1500) {
  await page.goto(base + path, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(400)
  // headless Chrome freezes CSS fade-ins: show everything at once
  await page.addStyleTag({ content: '*, *::before, *::after { transition: none !important; animation: none !important; }' }).catch(() => {})
  await page.evaluate(() => document.fonts.ready)
  await page.waitForTimeout(wait)
}
async function shot(page, name) { if (shots && (!only || only.split(',').includes(name))) await page.screenshot({ path: dir + name + '.png' }) }
const api = async (page, path) => page.evaluate(p => fetch('/api/v1' + p).then(r => r.json()), path)

// ---- desktop ----
{
  const { ctx, page } = await session()
  await go(page, '/')
  assert.ok((await page.content()).includes('Coming up'), 'Home shows the coming games')
  await shot(page, 'desktop-home')
  const groups = await api(page, '/groups')
  const tennis = groups.find(g => g.sport === 'tennis'), football = groups.find(g => g.sport === 'futsal'), padel = groups.find(g => g.sport === 'padel')
  await go(page, `/groups/${tennis.id}?tab=table`)
  assert.ok((await page.content()).includes('Rating'), 'the table has ratings')
  await shot(page, 'desktop-table')
  await go(page, `/groups/${football.id}`)
  await shot(page, 'desktop-group')
  const fg = (await api(page, `/games?group=${football.id}&upcoming=1`))[0]
  await go(page, `/games/${fg.id}`, 2000)
  assert.ok((await page.content()).includes('Who&#x27;s coming') || (await page.content()).includes("Who's coming"), 'a game shows who is coming')
  await shot(page, 'desktop-game')
  await page.getByRole('button', { name: /Pick fair teams/ }).first().click()
  await page.waitForTimeout(800)
  await shot(page, 'desktop-teams')
  await go(page, `/groups/${tennis.id}?tab=chat`)
  await shot(page, 'desktop-chat')
  const pg = (await api(page, `/games?group=${padel.id}&upcoming=1`))[0]
  await go(page, `/games/${pg.id}`)
  assert.ok((await page.content()).includes('Which times suit you'), 'a vote shows the times')
  await shot(page, 'desktop-vote')
  await go(page, `/groups/${tennis.id}?tab=people`)
  await shot(page, 'desktop-people')
  const ana = (await api(page, `/groups/${tennis.id}`)).members.find(m => m.name.startsWith('Marko'))
  await go(page, `/groups/${tennis.id}/players/${ana.user_id}`)
  await shot(page, 'desktop-player')
  await go(page, '/settings/apps')
  await shot(page, 'desktop-settings')
  await page.goto(base + '/')
  await page.keyboard.press('n')
  await page.waitForTimeout(600)
  await shot(page, 'desktop-plan')
  await ctx.close()
}

// ---- phone ----
{
  const { ctx, page } = await session({ phone: true })
  const groups = await api(page, '/groups')
  const tennis = groups.find(g => g.sport === 'tennis'), football = groups.find(g => g.sport === 'futsal')
  await go(page, '/')
  await shot(page, 'phone-home')
  const fg = (await api(page, `/games?group=${football.id}&upcoming=1`))[0]
  await go(page, `/games/${fg.id}`)
  await shot(page, 'phone-game')
  await go(page, `/groups/${tennis.id}?tab=table`)
  await shot(page, 'phone-table')
  await go(page, `/groups/${tennis.id}?tab=chat`)
  await shot(page, 'phone-chat')
  await ctx.close()
}
{
  const { ctx, page } = await session({ phone: true, dark: true })
  const groups = await api(page, '/groups')
  const tennis = groups.find(g => g.sport === 'tennis')
  await go(page, `/groups/${tennis.id}`)
  await shot(page, 'phone-group-dark')
  await ctx.close()
}

await browser.close()
if (errors.length) { console.error(errors.join('\n')); process.exit(1) }
console.log('screenshots ok')
