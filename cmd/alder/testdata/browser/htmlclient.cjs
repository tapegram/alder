// htmlclient.cjs BASE: an <html> attribute set in the browser survives boosted navigation
// (#189). The blog at BASE (served by `alder run`) has the scaffold's Layout, whose head script
// sets the class dark on <html> from localStorage or the system preference; the server renders
// a bare <html>. With a dark preference, a boosted link and a Back must keep the class, since the
// attrs event removes only what the server set. cmd/alder/patterns_browser_test.go
// (TestBrowser_ClientHtmlClassKept) runs it and reads the JSON it prints:
// {"checks": [{"name", "ok", "got"}...]}.
const { chromium } = require('playwright');

const [BASE] = process.argv.slice(2);
const checks = [];
const check = (name, ok, got) => checks.push({ name, ok: !!ok, got: JSON.stringify(got) });

(async () => {
  const browser = await chromium.launch();
  try {
    const ctx = await browser.newContext({ colorScheme: 'dark' });
    const page = await ctx.newPage();
    const errors = [];
    page.on('pageerror', (e) => errors.push(String(e)));
    const root = () => page.evaluate(() => ({
      path: location.pathname, cls: document.documentElement.className,
      attrs: Array.from(document.documentElement.attributes).map((a) => a.name), marker: window.__m === 1,
    }));
    await page.goto(BASE + '/');
    await page.waitForFunction(() => typeof window._alderO === 'function');
    let s = await root();
    check('dark from the head script on the full load', s.cls === 'dark', s);
    await page.evaluate(() => {
      window.__m = 1;
      const a = document.createElement('a');
      a.id = 'go'; a.href = '/login'; a.textContent = 'go';
      document.body.prepend(a);
    });
    await page.click('#go');
    await page.waitForFunction(() => location.pathname === '/login' && document.title === 'Log in', null, { timeout: 5000 }).catch(() => {});
    await page.waitForFunction(() => !document.documentElement.hasAttribute('data-alder-pending'), null, { timeout: 5000 }).catch(() => {});
    s = await root();
    check('boosted link: dark kept, no reload', s.path === '/login' && s.cls === 'dark' && s.marker, s);
    await page.goBack({ waitUntil: 'commit' }).catch(() => {});
    await page.waitForFunction(() => location.pathname === '/' && document.title === 'Blog', null, { timeout: 5000 }).catch(() => {});
    await page.waitForTimeout(300);
    s = await root();
    check('Back: dark kept, no reload', s.path === '/' && s.cls === 'dark' && s.marker, s);
    check('no page errors', errors.length === 0, errors);
  } finally {
    await browser.close();
  }
  process.stdout.write(JSON.stringify({ checks }));
})().catch((e) => {
  process.stderr.write(String(e && e.stack || e));
  process.exit(1);
});
