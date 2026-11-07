// README screenshots and a browser smoke test, against a demo instance (DEMO_DATA=true seeds a small landlord's year):
//
//   DATA_DIR=/tmp/rentals-demo DEMO_DATA=true ADMIN_EMAIL=demo@example.com ADMIN_PASSWORD=demodemo1 ADMIN_NAME=Aleksandar PORT=18253 ./pocket-rentals
//   CDP=http://127.0.0.1:9343 BASE=http://127.0.0.1:18253 node scripts/screenshots.mjs   # any Chrome with remote debugging
//   SHOTS=0 … node scripts/screenshots.mjs                                                # checks only
// playwright-core comes from web/node_modules, or set PW_FROM to a package.json next to an install of it.
import { createRequire } from 'node:module'
import assert from 'node:assert/strict'

const require = createRequire(process.env.PW_FROM || new URL('../web/package.json', import.meta.url))
let pw
try { pw = require('playwright') } catch { pw = require('playwright-core') }
const { chromium } = pw
const base = process.env.BASE || 'http://127.0.0.1:18253'
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
  await page.goto(base + path, { waitUntil: 'networkidle' })
  // headless Chrome freezes CSS fade-ins: show everything at once
  await page.addStyleTag({ content: '*, *::before, *::after { transition: none !important; animation: none !important; }' }).catch(() => {})
  await page.evaluate(() => document.fonts.ready)
  await page.waitForTimeout(wait)
}
async function shot(page, name) { if (shots && (!only || only.split(',').includes(name))) await page.screenshot({ path: dir + name + '.png' }) }
const api = async (page, path) => page.evaluate(p => fetch('/api/v1' + p).then(r => r.json()), path)
const firstID = async (page, path, pick = () => true) => (await api(page, path)).filter(pick)[0].id

// ---- desktop ----
{
  const { ctx, page } = await session()
  await go(page, '/', 1800)
  assert.ok((await page.content()).includes('Rent this month'), 'Today shows the totals')
  await shot(page, 'desktop-today')
  await go(page, '/rent', 1200)
  assert.ok((await page.content()).includes('Came in'), 'the rent month')
  await shot(page, 'desktop-rent')
  await go(page, '/properties', 1200)
  await shot(page, 'desktop-places')
  const pid = await firstID(page, '/properties', p => p.kind === 'building')
  await go(page, `/properties/${pid}`, 1500)
  await shot(page, 'desktop-place')
  const lid = await firstID(page, '/leases', l => l.overdue > 0)
  await go(page, `/leases/${lid}`, 2000)
  assert.ok((await page.content()).includes('Rent and charges'), 'a lease shows its rent')
  await shot(page, 'desktop-lease')
  await page.getByRole('button', { name: /Rent came in/ }).first().click()
  await page.waitForTimeout(700)
  await shot(page, 'desktop-payment')
  await page.keyboard.press('Escape')
  await go(page, '/repairs', 1200)
  await shot(page, 'desktop-repairs')
  const rid = await firstID(page, '/requests', r => r.priority === 'urgent' && r.status !== 'done')
  await go(page, `/repairs/${rid}`, 1500)
  await shot(page, 'desktop-repair')
  const iid = await firstID(page, '/inspections', i => i.kind === 'move_out')
  await go(page, `/inspections/${iid}`, 1500)
  assert.ok((await page.content()).includes('Before:'), 'moving out is compared with moving in')
  await shot(page, 'desktop-inspection')
  await go(page, '/tenants', 1000)
  await shot(page, 'desktop-tenants')
  await go(page, '/costs', 1000)
  await shot(page, 'desktop-costs')
  await go(page, '/reports', 1500)
  await shot(page, 'desktop-reports')
  await go(page, '/settings/rent', 1200)
  await shot(page, 'desktop-settings-rent')
  await go(page, '/settings/apps', 1500)
  await shot(page, 'desktop-settings-apps')
  await go(page, '/properties', 800)
  await page.keyboard.press('a')
  await page.waitForTimeout(600)
  await shot(page, 'desktop-new-place')
  await ctx.close()
}

// ---- phone ----
{
  const { ctx, page } = await session({ phone: true })
  await go(page, '/', 1800)
  await shot(page, 'phone-today')
  await go(page, '/rent', 1200)
  await shot(page, 'phone-rent')
  const lid = await firstID(page, '/leases', l => l.overdue > 0)
  await go(page, `/leases/${lid}`, 2000)
  await shot(page, 'phone-lease')
  const iid = await firstID(page, '/inspections', i => i.kind === 'move_out')
  await go(page, `/inspections/${iid}`, 1500)
  await shot(page, 'phone-inspection')
  await go(page, '/properties', 1200)
  await shot(page, 'phone-places')
  await go(page, '/', 800)
  await page.getByRole('button', { name: 'Menu ([)' }).first().click().catch(() => page.locator('.topbar .btn').first().click())
  await page.waitForTimeout(500)
  await shot(page, 'phone-menu')
  await ctx.close()
}

// ---- dark ----
{
  const { ctx, page } = await session({ dark: true })
  await go(page, '/', 1500)
  await shot(page, 'desktop-today-dark')
  const lid = await firstID(page, '/leases', l => l.overdue > 0)
  await go(page, `/leases/${lid}`, 1800)
  await shot(page, 'desktop-lease-dark')
  await ctx.close()
}

await browser.close()
if (errors.length) { console.error(errors.join('\n')); process.exit(1) }
console.log('ok')
