// theme.cjs BASE APPDIR: the theme configurator at /_dev/theme on the blog served by
// `alder run` at BASE (APPDIR its source). cmd/alder/theme_ui_browser_test.go runs it and reads
// the JSON it prints: {"checks": [{"name", "ok", "got"}...]}. With ALDER_SHOTS_DIR set, it saves
// theme-ui-light.png and theme-ui-dark.png there.
const { chromium } = require('playwright');
const fs = require('fs');
const path = require('path');

const [BASE, APPDIR] = process.argv.slice(2);
const SHOTS = process.env.ALDER_SHOTS_DIR;
const checks = [];
const check = (name, ok, got) => checks.push({ name, ok: !!ok, got: JSON.stringify(got) });

// the child's build generation, from the reload subscriber the router writes into every page
const generation = async () => {
  const body = await (await fetch(BASE + '/')).text();
  const m = /\/_dev\/reload\?b=([0-9a-f]{8}-[0-9]+)/.exec(body);
  return m ? m[1] : null;
};

(async () => {
  const browser = await chromium.launch();
  try {
    const ctx = await browser.newContext({ viewport: { width: 1280, height: 900 }, colorScheme: 'light' });
    const page = await ctx.newPage();
    const errors = [];
    page.on('pageerror', (e) => errors.push(String(e)));
    const open = async () => {
      await page.waitForSelector('[data-field="primary"] input.value', { timeout: 30000 });
      await page.waitForFunction(() => !document.getElementById('tp-status').hasAttribute('data-busy'), null, { timeout: 60000 });
    };
    await page.goto(BASE + '/_dev/theme');
    await open();
    const buttonBg = () => page.evaluate(() => getComputedStyle(document.querySelector('#g-primary > button')).backgroundColor);
    // a colour as the browser computes it, for comparing with a computed background
    const computed = (v) => page.evaluate((c) => { const e = document.createElement('div'); e.style.background = c; document.body.append(e);
      const bg = getComputedStyle(e).backgroundColor; e.remove(); return bg; }, v);
    const setScheme = async (s) => { if (await page.getAttribute('#scheme', 'data-scheme') !== s) await page.click('#scheme'); };

    if (SHOTS) {
      await page.screenshot({ path: path.join(SHOTS, 'theme-ui-light.png'), fullPage: false });
    }

    // primary-live: the gallery's button follows the control, with no rebuild
    await setScheme('light');
    const gen0 = await generation();
    await page.fill('[data-field="primary"] input.value', '#ff0000');
    await page.waitForTimeout(300);
    const bg = await buttonBg();
    const inline = await page.evaluate(() => document.documentElement.style.getPropertyValue('--primary'));
    const gen1 = await generation();
    check('primary-live', bg === 'rgb(255, 0, 0)' && inline === '#ff0000' && gen0 && gen0 === gen1, { bg, inline, gen0, gen1 });

    // dark-switch: <html class="dark"> and the dark scheme's colours in the controls
    await page.click('#scheme');
    await page.waitForTimeout(300); // the buttons' colour transition
    const dark = await page.evaluate(() => ({ cls: document.documentElement.classList.contains('dark'),
      scheme: document.getElementById('scheme').getAttribute('data-scheme'), heading: document.getElementById('tp-colours-h').textContent,
      primary: document.querySelector('[data-field="primary"] input.value').value }));
    check('dark-switch', dark.cls && dark.scheme === 'dark' && /dark/.test(dark.heading) && dark.primary === 'oklch(0.68 0.16 277)', dark);
    if (SHOTS) {
      await page.emulateMedia({ colorScheme: 'dark' });
      await page.waitForTimeout(200);
      await page.screenshot({ path: path.join(SHOTS, 'theme-ui-dark.png'), fullPage: false });
      await page.emulateMedia({ colorScheme: 'light' });
    }

    // clear-shows-default: clearing dark primary (which the committed theme sets) shows
    // alder-ui's dark default, set inline, not the committed value
    const st = await page.evaluate(() => fetch('/_dev/theme/state').then((r) => r.json()));
    const darkDefault = st.defaults.primary.Dark;
    const committedBg = await buttonBg();
    await page.click('[data-field="primary"] .clear');
    await page.waitForTimeout(300);
    const clearedBg = await buttonBg();
    const want = await computed(darkDefault);
    const cleared = await page.evaluate(() => ({ inline: document.documentElement.style.getPropertyValue('--primary'),
      text: document.querySelector('[data-field="primary"] input.value').value,
      placeholder: document.querySelector('[data-field="primary"] input.value').placeholder }));
    check('clear-shows-default', clearedBg === want && clearedBg !== committedBg && cleared.inline === darkDefault && cleared.text === '' && cleared.placeholder === darkDefault,
      { clearedBg, want, committedBg, darkDefault, cleared });

    // write-rebuilds: App.alder gets the light primary and loses the dark one; the rebuild
    // reloads the page, which starts from the new committed theme with nothing to write
    const writeEnabled = await page.evaluate(() => !document.getElementById('write').disabled);
    const reloaded = page.waitForEvent('load', { timeout: 120000 });
    await page.click('#write');
    await reloaded;
    await open();
    await page.waitForFunction(() => document.getElementById('write').disabled && !document.getElementById('tp-status').hasAttribute('data-busy'), null, { timeout: 60000 });
    const src = fs.readFileSync(path.join(APPDIR, 'app', 'App.alder'), 'utf8');
    const decl = (/theme \{[\s\S]*?\n\}/.exec(src) || [''])[0];
    await setScheme('light');
    const after = await page.evaluate(() => ({ write: document.getElementById('write').disabled,
      changed: document.querySelectorAll('.tp-row.changed').length,
      primary: document.querySelector('[data-field="primary"] input.value').value,
      inline: document.documentElement.style.cssText }));
    const bgAfter = await buttonBg();
    const gen2 = await generation();
    check('write-rebuilds', writeEnabled && /light: \{[^}]*primary: "#ff0000"/.test(decl) && !/dark: \{[^}]*primary: /.test(decl) && gen2 !== gen1 &&
      after.write && after.changed === 0 && after.primary === '#ff0000' && bgAfter === 'rgb(255, 0, 0)', { writeEnabled, decl, gen1, gen2, after, bgAfter });

    // iframe-follows: the App tab's <html> carries the working theme, also after a navigation in it
    await page.fill('[data-field="primary"] input.value', '#00ff00');
    await page.click('#scheme'); // dark
    await page.click('#tab-app');
    const frameEl = await page.waitForSelector('iframe#app');
    await page.waitForFunction(() => { const d = document.getElementById('app').contentDocument; return d && d.readyState === 'complete' && d.querySelector('body') && d.documentElement.classList.contains('dark'); }, null, { timeout: 30000 });
    const frame = await frameEl.contentFrame();
    const nav = frame.waitForNavigation({ timeout: 30000 });
    await frame.evaluate(() => { location.href = '/?theme-test=1'; });
    await nav;
    await page.waitForFunction(() => { const d = document.getElementById('app').contentDocument; return d && d.readyState === 'complete' && /theme-test/.test(d.location.search) && d.documentElement.classList.contains('dark'); }, null, { timeout: 30000 });
    const fr = await page.evaluate(() => { const h = document.getElementById('app').contentDocument.documentElement;
      return { cls: h.className, light: h.style.getPropertyValue('--primary'), radius: h.style.getPropertyValue('--radius'), top: document.documentElement.style.cssText === h.style.cssText }; });
    // the dark scheme shows dark primary: the working theme has none (cleared and written), so
    // the stylesheets decide; switch back to light to see #00ff00 follow
    await page.click('#scheme');
    await page.waitForTimeout(200);
    const fr2 = await page.evaluate(() => { const h = document.getElementById('app').contentDocument.documentElement;
      return { cls: h.className, primary: h.style.getPropertyValue('--primary') }; });
    check('iframe-follows', /dark/.test(fr.cls) && fr.top && !/dark/.test(fr2.cls) && fr2.primary === '#00ff00', { fr, fr2 });

    // diag-names-scheme: a refused dark value, shown while Light is selected, says it is dark's
    await page.click('#tab-gallery');
    await setScheme('dark');
    await page.fill('[data-field="ring"] input.value', 'oklch(1 2');
    await setScheme('light');
    await page.click('#write');
    await page.waitForFunction(() => document.querySelector('[data-field="ring"] .tp-diag').textContent !== '', null, { timeout: 10000 });
    const ringDiag = await page.textContent('[data-field="ring"] .tp-diag');
    await setScheme('dark');
    const ringDiagDark = await page.textContent('[data-field="ring"] .tp-diag');
    check('diag-names-scheme', /^dark: /.test(ringDiag) && !/^dark: /.test(ringDiagDark) && /ring/.test(ringDiagDark), { ringDiag, ringDiagDark });

    // import-copy: importing the tweakcn fixture lists the unplaced block with a copy button
    await page.click('#reset');
    await page.click('#import');
    await page.fill('#import-css', fs.readFileSync(path.join(__dirname, '..', '..', '..', '..', 'internal', 'theme', 'testdata', 'import', 'tweakcn.css'), 'utf8'));
    await page.click('#import-go');
    await page.waitForSelector('#tp-import-out pre', { timeout: 10000 });
    const imp = await page.evaluate(() => ({ copy: document.getElementById('import-copy')?.textContent, pre: document.querySelector('#tp-import-out pre').textContent,
      write: document.getElementById('write').disabled }));
    check('import-copy', imp.copy === 'Copy' && /--font-serif/.test(imp.pre) && !imp.write, imp);

    check('no page errors', errors.length === 0, errors);
  } finally {
    await browser.close();
  }
  console.log(JSON.stringify({ checks }));
})().catch((e) => { console.error(e && e.stack || e); process.exit(1); });
