// errors.cjs BASE APPDIR: compile errors in the dev panel, against todos served by `alder run`
// at BASE (APPDIR is its source). cmd/alder/devui_browser_test.go runs it and reads the JSON it
// prints: {"checks": [{"name", "ok", "got"}...]}. It breaks app/Table/Todo.alder, expects the strip
// and the panel's Errors tab (and no movement of the page), then fixes it.
const { chromium } = require('playwright');
const fs = require('fs');
const path = require('path');

const [BASE, APPDIR] = process.argv.slice(2);
const checks = [];
const check = (name, ok, got) => checks.push({ name, ok: !!ok, got: JSON.stringify(got) });

(async () => {
  const browser = await chromium.launch();
  const file = path.join(APPDIR, 'app', 'Table', 'Todo.alder');
  const good = fs.readFileSync(file, 'utf8');
  try {
    const page = await (await browser.newContext({ viewport: { width: 900, height: 600 } })).newPage();
    await page.goto(BASE + '/login');
    await page.waitForSelector('al-devpanel');
    const layout = () => page.evaluate(() => {
      const r = (document.querySelector('main') || document.body).getBoundingClientRect();
      const h = document.querySelector('h1,h2,form');
      return { top: Math.round(h.getBoundingClientRect().top), left: Math.round(h.getBoundingClientRect().left),
        mainHeight: Math.round(r.height), mainWidth: Math.round(r.width), main: Math.round(r.top) };
    });
    const before = await layout();
    check('no strip and no Errors tab on a good build', await page.evaluate(() =>
      !document.querySelector('._alder-strip') && !document.querySelector('al-devpanel button[data-errors]')), null);

    // break the app: a type error
    fs.writeFileSync(file, good.replace('|> List.length\n', '|> List.length |> Text.length\n'));
    await page.waitForSelector('._alder-strip', { timeout: 120000 });
    await page.waitForSelector('al-devpanel button[data-errors]');
    const strip = await page.evaluate(() => {
      const s = document.querySelector('._alder-strip'), cs = getComputedStyle(s), r = s.getBoundingClientRect();
      return { text: s.textContent, position: cs.position, top: Math.round(r.top), width: Math.round(r.width), vw: innerWidth };
    });
    check('strip: fixed at the top, full width', strip.position === 'fixed' && strip.top === 0 && strip.width === strip.vw, strip);
    check('strip: says what failed', /Build failed: 1 error\. The page is the last good build\./.test(strip.text) && /Show errors/.test(strip.text), strip.text);
    const after = await layout();
    check('layout does not move when the strip appears', JSON.stringify(before) === JSON.stringify(after), { before, after });

    const tab = await page.evaluate(() => {
      const p = document.querySelector('al-devpanel'), btns = [...p.querySelectorAll(':scope > nav > button')];
      const sec = p.querySelector(':scope > section#_alder_tab_errors');
      return { first: btns[0].textContent.trim(), selected: btns[0].getAttribute('aria-selected'), open: p.hasAttribute('open'),
        secVisible: !!sec && !sec.hidden && getComputedStyle(sec).display !== 'none',
        text: sec ? sec.innerText : '', others: [...p.querySelectorAll(':scope > section')].filter((s) => s.id !== '_alder_tab_errors').every((s) => s.hidden),
        listInFlow: getComputedStyle(document.querySelector('#_alder_errors ._alder-errs')).display };
    });
    check('Errors tab: first, with a count badge, selected, panel open', /^Errors\s*1$/.test(tab.first) && tab.selected === 'true' && tab.open, tab);
    check('Errors tab: the other sections are hidden, the page holds no list', tab.secVisible && tab.others && tab.listInFlow === 'none', tab);
    check('Errors tab: file:line:col, code, message', /app\/Table\/Todo\.alder:36:3/.test(tab.text) && /E0003/.test(tab.text) && /expected Text, got Int/.test(tab.text), tab.text);
    check('Errors tab: the excerpt with the caret', /\n {2}todos\n {2}\^/.test(tab.text), tab.text);

    // a boosted navigation keeps the strip, the tab and its selection
    await page.evaluate(() => {
      const a = document.createElement('a');
      a.id = 'go'; a.href = '/login?x=1'; a.textContent = 'go';
      a.style.cssText = 'position:fixed;bottom:60px;left:0;z-index:99;background:#fff';
      document.body.prepend(a);
    });
    const rid = await page.evaluate(() => document.querySelector('al-devpanel > nav').getAttribute('data-rid'));
    await page.click('#go');
    await page.waitForFunction((r) => document.querySelector('al-devpanel > nav').getAttribute('data-rid') !== r, rid, { timeout: 10000 });
    await page.waitForTimeout(300);
    const morphed = await page.evaluate(() => ({
      strip: !!document.querySelector('._alder-strip'),
      selected: document.querySelector('al-devpanel > nav > button[data-errors]')?.getAttribute('aria-selected'),
      shown: document.querySelector('al-devpanel > section#_alder_tab_errors')?.hidden === false,
      tab: document.querySelector('al-devpanel').getAttribute('data-tab'), nav: document.querySelector('al-devpanel > nav').textContent,
    }));
    check('after a morph: the strip and the Errors tab survive, still selected', morphed.strip && morphed.selected === 'true' && morphed.shown, morphed);

    // the strip's button opens the tab again after another was chosen
    await page.click('al-devpanel > nav > button:has-text("SQL")');
    await page.click('._alder-strip button');
    check('the strip button opens the Errors tab', await page.evaluate(() => !document.querySelector('al-devpanel > section#_alder_tab_errors').hidden), null);

    // fix it: both go away with the reload
    fs.writeFileSync(file, good);
    await page.waitForFunction(() => !document.querySelector('._alder-strip') && !document.querySelector('al-devpanel button[data-errors]'), null, { timeout: 120000 });
    await page.waitForSelector('al-devpanel');
    check('a good build clears the strip and the tab', await page.evaluate(() =>
      !document.querySelector('._alder-strip') && !document.querySelector('al-devpanel button[data-errors]') &&
      [...document.querySelectorAll('al-devpanel > section')].some((s) => !s.hidden || !document.querySelector('al-devpanel').hasAttribute('open'))), null);
    const final = await layout();
    check('layout is where it was', final.top === before.top && final.mainHeight === before.mainHeight, { before, final });
  } finally {
    fs.writeFileSync(file, good);
    await browser.close();
  }
  console.log(JSON.stringify({ checks }));
})().catch((e) => { console.error(e && e.stack || e); process.exit(1); });
