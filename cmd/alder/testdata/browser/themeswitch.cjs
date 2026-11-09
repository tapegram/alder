// themeswitch.cjs BASE: the theme switch of book chapter 6 (#189), the corpus app
// routes-theme-ok served by `alder run` at BASE. Its button posts SwitchTheme, which flips the
// theme in the session and answers `refresh`; the layout puts the theme on <html> as the class
// dark. Under Datastar the class must change in place: a marker set on window before the click
// survives, so the page was not reloaded. The second press removes the server's class, and an
// attribute set in the browser stays. cmd/alder/patterns_browser_test.go
// (TestBrowser_ThemeSwitch) runs it and reads the JSON it prints:
// {"checks": [{"name", "ok", "got"}...]}.
const { chromium } = require('playwright');

const [BASE] = process.argv.slice(2);
const checks = [];
const check = (name, ok, got) => checks.push({ name, ok: !!ok, got: JSON.stringify(got) });

(async () => {
  const browser = await chromium.launch();
  try {
    const page = await browser.newPage();
    const errors = [];
    page.on('pageerror', (e) => errors.push(String(e)));
    await page.goto(BASE + '/');
    await page.waitForFunction(() => typeof window._alderO === 'function');
    const dark = () => page.evaluate(() => document.documentElement.classList.contains('dark'));
    check('light at first', !(await dark()), await page.evaluate(() => document.documentElement.className));
    // an attribute set in the browser: the attrs event removes only what the server set
    await page.evaluate(() => { window._themeMarker = 1; document.documentElement.setAttribute('data-client', '1'); });

    const press = async (want, label) => {
      await page.click('#switch');
      try {
        await page.waitForFunction((w) => document.documentElement.classList.contains('dark') === w, want, { timeout: 5000 });
      } catch (e) {
        // the check below reports it
      }
      await page.waitForFunction((l) => document.getElementById('switch').textContent === l, label, { timeout: 5000 }).catch(() => {});
      check(`dark is ${want} after the press`, (await dark()) === want, await page.evaluate(() => document.documentElement.className));
      check(`button reads ${label}`, (await page.textContent('#switch')) === label, await page.textContent('#switch'));
      check(`no reload (press to ${want})`, await page.evaluate(() => window._themeMarker === 1), await page.evaluate(() => window._themeMarker));
    };
    await press(true, 'Light mode');
    await press(false, 'Dark mode');
    check('an attribute set in the browser stays', await page.evaluate(() => document.documentElement.getAttribute('data-client') === '1'),
      await page.evaluate(() => Array.from(document.documentElement.attributes).map((a) => a.name)));
    check('no page errors', errors.length === 0, errors);
  } finally {
    await browser.close();
  }
  process.stdout.write(JSON.stringify({ checks }));
})().catch((e) => {
  process.stderr.write(String(e && e.stack || e));
  process.exit(1);
});
