// panel.cjs BASE APPDIR: the dev panel's resize handle, minimise button and Ctrl+Shift+D, and
// that its state (open or minimised, height, last tab) survives a morph and a reload, against
// todos served by `alder run` at BASE. cmd/alder/devui_browser_test.go runs it and reads the JSON
// it prints: {"checks": [{"name", "ok", "got"}...]}.
const { chromium } = require('playwright');

const [BASE] = process.argv.slice(2);
const checks = [];
const check = (name, ok, got) => checks.push({ name, ok: !!ok, got: JSON.stringify(got) });
const VH = 600;

(async () => {
  const browser = await chromium.launch();
  try {
    const ctx = await browser.newContext({ viewport: { width: 900, height: VH } });
    const page = await ctx.newPage();
    const errors = [];
    page.on('pageerror', (e) => errors.push(String(e)));
    await page.goto(BASE + '/login');
    await page.waitForSelector('al-devpanel > [data-grip]', { state: 'attached' });
    const info = () => page.evaluate(() => {
      const p = document.querySelector('al-devpanel'), sec = [...p.querySelectorAll(':scope > section')].find((s) => !s.hidden);
      const r = p.getBoundingClientRect(), cs = getComputedStyle(p);
      return { open: p.hasAttribute('open'), min: p.hasAttribute('data-min'), h: sec && getComputedStyle(sec).display !== 'none' ? Math.round(sec.getBoundingClientRect().height) : 0,
        position: cs.position, right: Math.round(innerWidth - r.right), bottom: Math.round(innerHeight - r.bottom), w: Math.round(r.width), bg: cs.backgroundColor,
        tab: p.getAttribute('data-tab'), text: p.innerText, scrollH: document.documentElement.scrollHeight,
        selected: p.querySelector(':scope > nav > button[aria-selected=true]')?.textContent.trim() };
    });
    const drag = async (toY) => {
      const g = await (await page.$('al-devpanel > [data-grip]')).boundingBox();
      await page.mouse.move(g.x + g.width / 2, g.y + g.height / 2);
      await page.mouse.down();
      await page.mouse.move(g.x + g.width / 2, toY, { steps: 8 });
      await page.mouse.up();
    };
    const stored = () => page.evaluate(() => { try { return JSON.parse(localStorage.getItem('_alder_panel_ui')); } catch (e) { return 'throws'; } });

    // closed at first: the one-line bar, no handle to drag
    let s = await info();
    check('closed: a bar, the handle is not shown', !s.open && !s.min && await page.evaluate(() => getComputedStyle(document.querySelector('al-devpanel > [data-grip]')).display === 'none'), s);

    // open with a tab, then resize by dragging the top edge
    await page.click('al-devpanel > nav > button:has-text("Trace")');
    s = await info();
    const h0 = s.h;
    check('opened: the section shows', s.open && h0 > 0 && s.selected === 'Trace', s);
    const g = await (await page.$('al-devpanel > [data-grip]')).boundingBox();
    const edge = await (await page.$('al-devpanel')).boundingBox();
    check('the handle is on the panel\'s top edge', Math.abs(g.y + g.height / 2 - edge.y) <= 6 && g.width >= edge.width - 2, { g, edge });
    await drag(g.y + g.height / 2 - 150);
    s = await info();
    check('drag up 150px: 150px taller', Math.abs(s.h - (h0 + 150)) <= 2, { h0, now: s.h });
    await drag(0);
    s = await info();
    check('drag to the top: clamped to 80% of the viewport', s.h === Math.floor(VH * 0.8) || s.h === Math.round(VH * 0.8), s.h);
    await drag(VH - 1);
    s = await info();
    check('drag to the bottom: clamped to 120px', s.h === 120, s.h);
    await drag(VH - 1 - 0 - 0); // stays 120
    await page.focus('al-devpanel > [data-grip]');
    await page.keyboard.press('ArrowUp');
    s = await info();
    check('the handle takes the arrow keys', s.h > 120 && s.h <= 160, s.h);
    const st1 = await stored();
    check('the height is stored', st1 && st1.h === s.h && st1.open === true && st1.tab === '_alder_tab_trace', st1);

    // minimise: a small tab at the bottom right; no space reserved
    const hKept = s.h;
    await page.click('al-devpanel > [data-minbtn]');
    s = await info();
    check('minimised: fixed in the bottom-right corner, small', s.min && s.position === 'fixed' && s.right <= 24 && s.bottom <= 24 && s.w < 300 && s.h === 0, s);
    check('minimised: shows the last request\'s time', /\d+(\.\d+)? ms/.test(s.text) && !/Trace/.test(s.text), s.text);
    check('minimised: reserves no space at the bottom of the page', s.scrollH <= VH, s.scrollH);
    await page.evaluate(() => document.querySelector('al-devpanel').setAttribute('data-errors', '2'));
    const badge = await page.evaluate(() => getComputedStyle(document.querySelector('al-devpanel'), '::after').content);
    check('minimised: the error badge', /2/.test(badge), badge);
    await page.evaluate(() => document.querySelector('al-devpanel').removeAttribute('data-errors'));
    const lightBg = s.bg;
    await page.emulateMedia({ colorScheme: 'dark' });
    const darkBg = (await info()).bg;
    await page.emulateMedia({ colorScheme: 'light' });
    check('the tab follows dark and light mode', lightBg !== darkBg, { lightBg, darkBg });

    // a reload keeps it minimised; clicking the tab restores height and tab
    await page.reload();
    await page.waitForSelector('al-devpanel > [data-grip]', { state: 'attached' });
    s = await info();
    check('reload: still minimised', s.min, s);
    await page.click('al-devpanel');
    s = await info();
    check('click the tab: restored with its height and tab', !s.min && s.open && s.h === hKept && s.selected === 'Trace', { s, hKept });

    // Ctrl+Shift+D toggles
    await page.keyboard.press('Control+Shift+D');
    s = await info();
    check('Ctrl+Shift+D minimises', s.min, s);
    await page.keyboard.press('Control+Shift+D');
    s = await info();
    check('Ctrl+Shift+D restores', !s.min && s.open && s.h === hKept, s);

    // a morph keeps all of it
    const link = (href) => page.evaluate((h) => {
      const a = document.createElement('a');
      a.id = 'go'; a.href = h; a.textContent = 'go';
      a.style.cssText = 'position:fixed;top:0;left:0;z-index:99;background:#fff';
      document.body.prepend(a);
    }, href);
    await link('/login?x=1');
    const rid = await page.evaluate(() => document.querySelector('al-devpanel > nav').getAttribute('data-rid'));
    await page.click('#go');
    await page.waitForFunction((r) => document.querySelector('al-devpanel > nav').getAttribute('data-rid') !== r, rid, { timeout: 10000 });
    await page.waitForTimeout(300);
    s = await info();
    check('after a morph: open, height and tab kept; handle and button re-added', s.open && !s.min && s.h === hKept && s.selected === 'Trace' &&
      await page.evaluate(() => !!document.querySelector('al-devpanel > [data-grip]') && !!document.querySelector('al-devpanel > [data-minbtn]')), s);
    await page.click('al-devpanel > [data-minbtn]');
    await link('/login?x=2');
    const rid2 = await page.evaluate(() => document.querySelector('al-devpanel > nav').getAttribute('data-rid'));
    await page.click('#go');
    await page.waitForFunction((r) => document.querySelector('al-devpanel > nav').getAttribute('data-rid') !== r, rid2, { timeout: 10000 });
    await page.waitForTimeout(300);
    s = await info();
    check('after a morph: minimised stays minimised', s.min && s.position === 'fixed', s);
    await page.click('al-devpanel');

    // a new browser tab of the same browser (same storage) starts as this one left it
    const page2 = await ctx.newPage();
    await page2.goto(BASE + '/login');
    await page2.waitForSelector('al-devpanel > [data-grip]', { state: 'attached' });
    const s2 = await page2.evaluate(() => { const p = document.querySelector('al-devpanel');
      return { open: p.hasAttribute('open'), sel: p.querySelector(':scope > nav > button[aria-selected=true]')?.textContent.trim(),
        h: Math.round([...p.querySelectorAll(':scope > section')].find((x) => !x.hidden).getBoundingClientRect().height) }; });
    check('state is per browser: another tab opens as left', s2.open && s2.sel === 'Trace' && s2.h === hKept, s2);
    await page2.close();

    // storage unavailable: every access throws; the panel still works
    const ctx2 = await browser.newContext({ viewport: { width: 900, height: VH } });
    await ctx2.addInitScript(() => {
      const boom = () => { throw new DOMException('denied', 'SecurityError'); };
      Object.defineProperty(window, 'localStorage', { get: boom, configurable: true });
    });
    const p3 = await ctx2.newPage();
    const errs3 = [];
    p3.on('pageerror', (e) => errs3.push(String(e)));
    await p3.goto(BASE + '/login');
    await p3.waitForSelector('al-devpanel > [data-grip]', { state: 'attached' });
    await p3.click('al-devpanel > nav > button:has-text("SQL")');
    const gb = await (await p3.$('al-devpanel > [data-grip]')).boundingBox();
    await p3.mouse.move(gb.x + gb.width / 2, gb.y + gb.height / 2);
    await p3.mouse.down();
    await p3.mouse.move(gb.x + gb.width / 2, gb.y - 100, { steps: 5 });
    await p3.mouse.up();
    await p3.click('al-devpanel > [data-minbtn]');
    const m3 = await p3.evaluate(() => document.querySelector('al-devpanel').hasAttribute('data-min'));
    await p3.click('al-devpanel');
    await p3.keyboard.press('Control+Shift+D');
    const m4 = await p3.evaluate(() => document.querySelector('al-devpanel').hasAttribute('data-min'));
    check('no storage: resize, minimise, restore and the shortcut still work, with no page error', m3 && m4 && errs3.length === 0, { m3, m4, errs3 });
    await ctx2.close();

    check('no page errors', errors.length === 0, errors);
  } finally {
    await browser.close();
  }
  console.log(JSON.stringify({ checks }));
})().catch((e) => { console.error(e && e.stack || e); process.exit(1); });
