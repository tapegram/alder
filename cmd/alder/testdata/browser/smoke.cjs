// smoke.cjs BASE: the boosted-navigation smoke test against the blog served at BASE, in a
// headless Chromium (Playwright). cmd/alder/browser_test.go runs it (go test -tags browser) and
// reads the JSON it prints: {"checks": [{"name", "ok", "got"}...]}. Every navigation here
// must happen without a document load: window.__m, set once, must survive them all.
const { chromium } = require('playwright');

const BASE = process.argv[2];
const checks = [];
const check = (name, ok, got) => checks.push({ name, ok: !!ok, got: JSON.stringify(got) });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

(async () => {
  const browser = await chromium.launch();
  try {
    const ctx = await browser.newContext({ viewport: { width: 800, height: 400 } });
    const page = await ctx.newPage();
    let docs = [];
    page.on('request', (r) => { if (r.resourceType() === 'document') docs.push(r.method() + ' ' + r.url()); });
    const state = () => page.evaluate(() => ({
      path: location.pathname, title: document.title, y: Math.round(scrollY), marker: window.__m === 1,
      h1: (document.querySelector('h1') || {}).textContent,
    }));
    const at = (path) => page.waitForFunction((p) => location.pathname === p, path, { timeout: 5000 }).catch(() => {});
    const link = (href) => page.evaluate((h) => {
      const a = document.createElement('a');
      a.id = 'go'; a.href = h; a.textContent = 'go';
      a.style.cssText = 'position:fixed;top:0;left:0;z-index:99;background:#fff';
      document.body.prepend(a);
    }, href);

    await page.goto(BASE + '/');
    await page.evaluate(() => { window.__m = 1; document.documentElement.style.minHeight = '4000px'; });
    await page.evaluate(() => scrollTo(0, 700));
    await sleep(400); // the scroll recorder's trailing call
    docs = [];

    // a boosted click: no document load; the URL, the title and the scroll change
    await link('/login');
    await page.click('#go');
    await at('/login');
    await sleep(200);
    let s = await state();
    check('boosted click: URL and title', s.path === '/login' && s.title === 'Log in', s);
    check('boosted click: no document load', docs.length === 0 && s.marker, docs);
    check('boosted click: scrolled to the top', s.y === 0, s.y);

    // Back and Forward each restore the scroll position the entry was left at
    await page.evaluate(() => scrollTo(0, 500));
    await sleep(400);
    await page.goBack({ waitUntil: 'commit' }).catch(() => {});
    await at('/');
    await page.waitForFunction(() => document.title === 'Blog', null, { timeout: 5000 }).catch(() => {});
    await sleep(300);
    s = await state();
    check('Back: the page and its scroll position', s.path === '/' && s.title === 'Blog' && Math.abs(s.y - 700) <= 2, s);
    await page.goForward({ waitUntil: 'commit' }).catch(() => {});
    await at('/login');
    await page.waitForFunction(() => document.title === 'Log in', null, { timeout: 5000 }).catch(() => {});
    await sleep(300);
    s = await state();
    check('Forward: the page and its scroll position', s.path === '/login' && s.title === 'Log in' && Math.abs(s.y - 500) <= 2, s);
    check('Back and Forward: no document load', docs.length === 0 && s.marker, docs);

    // the login form: its Redirect is morphed in, with the cookie
    await page.fill('input[name=email]', 'author@example.com');
    await page.fill('input[name=password]', 'password');
    await page.click('button[type=submit]');
    await at('/');
    await page.waitForFunction(() => document.title === 'Blog', null, { timeout: 5000 }).catch(() => {});
    await sleep(200);
    s = await state();
    const cookies = (await ctx.cookies()).map((c) => c.name);
    const out = await page.evaluate(() => document.body.innerText.includes('Log out'));
    check('login: redirect morphed in, signed in', s.path === '/' && s.title === 'Blog' && cookies.includes('alder_session') && out, { s, cookies, out });
    check('login: no document load', docs.length === 0 && s.marker, docs);

    // a link to a missing page: the 404 page morphed in, the URL recorded
    await link('/posts/no-such-post');
    await page.click('#go');
    await at('/posts/no-such-post');
    await page.waitForFunction(() => document.title === 'Not found', null, { timeout: 5000 }).catch(() => {});
    s = await state();
    check('404: morphed in', s.path === '/posts/no-such-post' && s.title === 'Not found' && s.h1 === 'Not found', s);
    check('404: no document load', docs.length === 0 && s.marker, docs);
  } finally {
    await browser.close();
  }
  console.log(JSON.stringify({ checks }));
})().catch((e) => { console.error(e && e.stack || e); process.exit(1); });
