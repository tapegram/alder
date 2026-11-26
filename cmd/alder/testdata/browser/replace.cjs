// replace.cjs BASE: what Datastar v1.0.0 does with a `Response.replace` answer (#309): one
// `datastar-patch-elements` event with no selector (mode outer) holding several roots, each
// matched to the page by its id, then `mode remove` events naming ids by datastar.IDSelector. The
// page at BASE has <p id="count">, a <ul> of <li id="a">, <li id="b">, <li id="c"> and
// <li id="3">, and a button that GETs BASE/replace, which answers roots for #count, an id the
// page lacks (#missing) and #a, then removes #c and #3 (an id starting with a digit, escaped).
// cmd/alder/browser_test.go (TestBrowser_DatastarReplace) runs it and reads the JSON it prints:
// {"checks": [{"name", "ok", "got"}...], "body": "<the body's innerHTML>"}.
const { chromium } = require('playwright');

const [BASE] = process.argv.slice(2);
const checks = [];
const check = (name, ok, got) => checks.push({ name, ok: !!ok, got: JSON.stringify(got) });

(async () => {
  const browser = await chromium.launch();
  let body = '';
  try {
    const page = await browser.newPage();
    const errors = [];
    page.on('pageerror', (e) => errors.push(String(e)));
    await page.goto(BASE + '/');
    const answered = page.waitForResponse((r) => r.url().includes('/replace'));
    await page.click('#go');
    await answered;
    await page.waitForFunction(() => !document.getElementById('c'), null, { timeout: 5000 }).catch(() => {});
    await page.waitForTimeout(200);
    body = await page.$eval('body', (b) => b.innerHTML.replace(/\s+/g, ''));
    const text = (sel) => page.$eval(sel, (e) => e.textContent).catch(() => null);
    check('the first root replaces #count', (await text('#count')) === '2 rows', await text('#count'));
    check('the second root replaces #a, past a root the page lacks', (await text('#a')) === 'A', await text('#a'));
    check('a root the page lacks is not added', (await page.$('#missing')) === null, body);
    check('#b is left alone', (await text('#b')) === 'b', await text('#b'));
    check('the removal removes #c', (await page.$('#c')) === null, body);
    check('an id starting with a digit is removed by its escaped selector', (await page.$('[id="3"]')) === null, body);
    check('the list is what remains, in order', /<ulid="list"><liid="a">A<\/li><liid="b">b<\/li><\/ul>/.test(body), body);
    check('no page errors', errors.length === 0, errors);
  } finally {
    await browser.close();
  }
  process.stdout.write(JSON.stringify({ checks, body }));
})().catch((e) => {
  process.stderr.write(String(e && e.stack || e));
  process.exit(1);
});
