// multiroot.cjs BASE: does Datastar v1.0.0 morph several top-level elements of one
// `datastar-patch-elements` event (selector #a, mode outer) into the page? The page at BASE is a
// <ul> holding <li id="a">old</li> and a button that GETs BASE/patch, which answers
// `<li id="a">1</li><li id="b">2</li>` onto #a. cmd/alder/browser_test.go
// (TestBrowser_DatastarMultiRoot) runs it and reads the JSON it prints:
// {"checks": [{"name", "ok", "got"}...], "list": "<the ul's innerHTML>"}.
const { chromium } = require('playwright');

const [BASE] = process.argv.slice(2);
const checks = [];
const check = (name, ok, got) => checks.push({ name, ok: !!ok, got: JSON.stringify(got) });

(async () => {
  const browser = await chromium.launch();
  let list = '';
  try {
    const page = await browser.newPage();
    const errors = [];
    page.on('pageerror', (e) => errors.push(String(e)));
    await page.goto(BASE + '/');
    const answered = page.waitForResponse((r) => r.url().includes('/patch'));
    await page.click('#go');
    await answered;
    await page.waitForTimeout(500);
    list = await page.$eval('#list', (ul) => ul.innerHTML.replace(/\s+/g, ''));
    check('the first root replaces #a', /<liid="a">1<\/li>/.test(list), list);
    check('the second root lands after it', list === '<liid="a">1</li><liid="b">2</li>', list);
    check('no page errors', errors.length === 0, errors);
  } finally {
    await browser.close();
  }
  process.stdout.write(JSON.stringify({ checks, list }));
})().catch((e) => {
  process.stderr.write(String(e && e.stack || e));
  process.exit(1);
});
