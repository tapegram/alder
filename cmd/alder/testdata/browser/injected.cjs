// injected.cjs BASE APPDIR: the UI the dev server and the runtime inject (the error strip and
// sheet, the minimised dev panel, the pending bar) under alder-ui v0.4's global base layer
// (Preflight), in a light page and with .dark on <html>, against todos served by `alder run` at
// BASE (APPDIR is its source). cmd/alder/devui_browser_test.go runs it and reads the JSON it
// prints: {"checks": [{"name", "ok", "got"}...]}. Each value is compared with a probe element
// that resolves the design system's own token on the same page. It breaks app/Table/Todo.alder
// (a type error, as errors.cjs does) and restores it on every path. With ALDER_SHOTS_DIR set it writes a
// screenshot of each state there.
const { chromium } = require('playwright');
const fs = require('fs');
const path = require('path');

const [BASE, APPDIR] = process.argv.slice(2);
const SHOTS = process.env.ALDER_SHOTS_DIR || '';
const checks = [];
const check = (name, ok, got) => checks.push({ name, ok: !!ok, got: JSON.stringify(got) });
const MODES = ['light', 'dark'];

(async () => {
  const browser = await chromium.launch();
  const file = path.join(APPDIR, 'app', 'Table', 'Todo.alder');
  const good = fs.readFileSync(file, 'utf8');
  const bad = good.replace('|> List.length\n', '|> List.length |> Text.length\n');
  if (bad === good) throw new Error(file + ': the line to break is gone');
  try {
    if (SHOTS) fs.mkdirSync(SHOTS, { recursive: true });
    const shot = async (page, mode, state) => {
      if (SHOTS) await page.screenshot({ path: path.join(SHOTS, `injected-${mode}-${state}.png`) });
    };
    // the token's computed value on this page, through a probe element
    const probe = (page, prop, value) => page.evaluate(([prop, value]) => {
      const d = document.createElement('div');
      d.style.setProperty(prop, value);
      document.body.appendChild(d);
      const v = getComputedStyle(d).getPropertyValue(prop);
      d.remove();
      return v;
    }, [prop, value]);

    const pages = {};
    for (const mode of MODES) {
      const ctx = await browser.newContext({ viewport: { width: 900, height: 600 } });
      if (mode === 'dark') {
        await ctx.addInitScript(() => {
          const on = () => document.documentElement && document.documentElement.classList.add('dark');
          on();
          document.addEventListener('DOMContentLoaded', on);
        });
      }
      const page = await ctx.newPage();
      pages[mode] = page;
      await page.goto(BASE + '/login');
      await page.waitForSelector('al-devpanel > [data-minbtn]', { state: 'attached' });
      check(`${mode}: <html> class`, await page.evaluate(() => document.documentElement.classList.contains('dark')) === (mode === 'dark'), mode);

      // the fonts are served and the face is the vendored one
      const fonts = await page.evaluate(async () => {
        const faces = await document.fonts.load('12px "Geist Mono"');
        return { loaded: faces.length, check: document.fonts.check('12px "Geist Mono"') };
      });
      check(`${mode}: Geist Mono is loaded`, fonts.loaded >= 1 && fonts.check, fonts);

      // the pending bar
      const primary = await probe(page, 'background-color', 'var(--primary)');
      await page.evaluate(() => document.documentElement.setAttribute('data-alder-pending', ''));
      const bar = await page.evaluate(() => {
        const cs = getComputedStyle(document.documentElement, '::before');
        return { bg: cs.backgroundColor, height: cs.height, position: cs.position };
      });
      await shot(page, mode, 'pending');
      await page.evaluate(() => document.documentElement.removeAttribute('data-alder-pending'));
      check(`${mode}: pending bar is --primary, 2px`, bar.bg === primary && bar.height === '2px' && bar.position === 'fixed', { bar, primary });

      // the minimised panel tab
      await page.click('al-devpanel > [data-minbtn]');
      await page.waitForSelector('al-devpanel[data-min]');
      const radius = await probe(page, 'border-top-left-radius', 'var(--radius-md)');
      const tab = await page.evaluate(() => {
        const cs = getComputedStyle(document.querySelector('al-devpanel[data-min]'));
        return { style: cs.borderTopStyle, width: cs.borderTopWidth, radius: cs.borderTopLeftRadius };
      });
      await shot(page, mode, 'panel-min');
      check(`${mode}: minimised panel: solid border, --radius-md`, tab.style === 'solid' && tab.width === '1px' && tab.radius === radius, { tab, radius });
    }

    // break the app: a type error
    fs.writeFileSync(file, bad);
    const sheets = {};
    for (const mode of MODES) {
      const page = pages[mode];
      await page.waitForSelector('._alder-strip', { timeout: 120000 });
      const destructive = await probe(page, 'background-color', 'var(--destructive)');
      const strip = await page.evaluate(() => {
        const s = getComputedStyle(document.querySelector('._alder-strip'));
        const b = getComputedStyle(document.querySelector('._alder-strip button'));
        return { font: s.fontFamily, bg: s.backgroundColor, lineHeight: s.lineHeight,
          button: { style: b.borderTopStyle, width: b.borderTopWidth, bg: b.backgroundColor, color: b.color, stripColor: s.color } };
      });
      await shot(page, mode, 'strip');
      check(`${mode}: strip is in Geist Mono`, strip.font.startsWith('"Geist Mono"'), strip.font);
      check(`${mode}: strip background is --destructive`, strip.bg === destructive, { strip: strip.bg, destructive });
      check(`${mode}: strip button: 1px solid border, transparent, the strip's colour`, strip.button.style === 'solid' && strip.button.width === '1px' &&
        strip.button.bg === 'rgba(0, 0, 0, 0)' && strip.button.color === strip.button.stripColor, strip.button);

      // text on --destructive is --background: white-ish on red in light, near-black on the
      // lighter salmon --destructive in dark (white there would fall below AA); not
      // --destructive-foreground, which alder-ui's dark theme makes a red
      const onDestructive = await probe(page, 'color', 'var(--background)');
      await page.waitForSelector('al-devpanel[data-min][data-errors]');
      const badge = await page.evaluate(() => {
        const cs = getComputedStyle(document.querySelector('al-devpanel[data-min]'), '::after');
        return { content: cs.content, bg: cs.backgroundColor, color: cs.color };
      });
      check(`${mode}: strip text and the minimised tab's error badge are var(--background) on --destructive${mode === 'dark' ? ' (dark: the strip text equals the probe of var(--background))' : ''}`, strip.button.stripColor === onDestructive &&
        badge.content === '"1"' && badge.bg === destructive && badge.color === onDestructive, { strip: strip.button.stripColor, badge, onDestructive, destructive });

      // the sheet: what the strip's button shows on a page without the dev panel
      await page.evaluate(() => document.getElementById('_alder_panel').remove());
      await page.click('._alder-strip button');
      await page.waitForSelector('#_alder_errors[data-show] ._alder-errs', { state: 'visible' });
      const background = await probe(page, 'background-color', 'var(--background)');
      const sheet = await page.evaluate(() => {
        const cs = getComputedStyle(document.querySelector('#_alder_errors[data-show] ._alder-errs'));
        const pre = getComputedStyle(document.querySelector('#_alder_errors ._alder-errs pre'));
        const msg = getComputedStyle(document.querySelector('#_alder_errors ._alder-errs ._alder-msg'));
        return { bg: cs.backgroundColor, color: cs.color, font: cs.fontFamily, border: cs.borderTopStyle + ' ' + cs.borderTopWidth,
          pre: pre.whiteSpace + ' ' + pre.marginTop, msg: msg.marginTop };
      });
      await shot(page, mode, 'sheet');
      sheets[mode] = sheet;
      check(`${mode}: sheet: --background, top border, Geist Mono, its own margins`, sheet.bg === background && sheet.border === 'solid 1px' &&
        sheet.font.startsWith('"Geist Mono"') && sheet.pre === 'pre 4px' && sheet.msg === '4px', { sheet, background });
    }
    check('dark: the sheet\'s background differs from light\'s', sheets.dark.bg !== sheets.light.bg, sheets);
  } finally {
    fs.writeFileSync(file, good);
    await browser.close();
  }
  console.log(JSON.stringify({ checks }));
})().catch((e) => { console.error(e && e.stack || e); process.exit(1); });
