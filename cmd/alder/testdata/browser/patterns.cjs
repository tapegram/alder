// patterns.cjs BASE: drives the four route-trigger patterns (#160) of the corpus app
// routes-patterns-ok, served at BASE behind the recording proxy of
// cmd/alder/patterns_browser_test.go (TestBrowser_Patterns), and prints
// {"errors": [one entry per failed check]}. The proxy answers GET /_patterns/loads with
// {loads: {path: count}, gets: [path?query, ...]}, POST /_patterns/reset forgets them, and
// POST /_patterns/delay holds the next /results answer 800 ms.
const { chromium } = require('playwright');

const [BASE] = process.argv.slice(2);
const errors = [];

function check(name, ok, got) {
  if (!ok) errors.push(`${name}: got ${typeof got === 'string' ? got : JSON.stringify(got)}`);
}

async function proxy(path, method) {
  const res = await fetch(BASE + path, { method: method || 'GET' });
  return method ? null : res.json();
}

const reset = () => proxy('/_patterns/reset', 'POST');
const loads = () => proxy('/_patterns/loads');
// queries is the value of q in each recorded GET of /results.
const queries = (seen) =>
  (seen.gets || []).filter((u) => u.startsWith('/results')).map((u) => new URL(u, BASE).searchParams.get('q'));

(async () => {
  const browser = await chromium.launch();
  try {
    // A short viewport, so the first sentinel is below the fold until the page is scrolled.
    const context = await browser.newContext({ viewport: { width: 800, height: 200 } });
    const page = await context.newPage();
    page.on('pageerror', (e) => errors.push(String(e)));
    const fresh = async () => {
      await reset();
      await page.goto(BASE + '/');
      await page.waitForFunction(() => typeof window._alderF === 'function');
    };

    // Slow widget and polling badge: one /slow on load; /badge once on load and once per
    // 1 s tick, never a second request per tick (data-init re-run after its own patch).
    await fresh();
    const loaded = Date.now();
    await page.waitForFunction(() => document.querySelector('#slow')?.textContent === 'Ready', null, { timeout: 5000 })
      .catch(() => {});
    check('slow: #slow patched', (await page.textContent('#slow')) === 'Ready', await page.textContent('#slow'));
    await page.waitForTimeout(2500 - (Date.now() - loaded));
    let seen = await loads();
    const elapsed = Date.now() - loaded;
    check('slow: one /slow request', seen.loads['/slow'] === 1, seen.loads);
    const ticks = Math.floor(elapsed / 1000);
    const badge = seen.loads['/badge'] || 0;
    check(`badge: 1 + ${ticks} ticks requests in ${elapsed} ms`, badge >= ticks && badge <= ticks + 1, seen.loads);
    check('badge: #badge patched', (await page.textContent('#badge')) === '0', await page.textContent('#badge'));
    await page.waitForTimeout(1000);
    const more = (await loads()).loads['/badge'] || 0;
    check('badge: one request in the next tick', more === badge + 1, { before: badge, after: more });

    // Search as you type: three quick keystrokes are one request, after the debounce.
    await fresh();
    await page.type('input[name=q]', 'mil', { delay: 40 });
    await page.waitForFunction(() => document.querySelector('#echo')?.textContent === 'mil', null, { timeout: 5000 })
      .catch(() => {});
    await page.waitForTimeout(500);
    seen = await loads();
    check('search: one /results request with q=mil', JSON.stringify(queries(seen)) === '["mil"]', seen.gets);
    check('search: #results patched', (await page.textContent('#echo')) === 'mil', await page.textContent('#echo'));
    check('search: matching words', JSON.stringify(await page.$$eval('#results .word', (ls) => ls.map((l) => l.textContent))) === '["milk","millet"]',
      await page.$$eval('#results .word', (ls) => ls.map((l) => l.textContent)));

    // The typed text reaches the server exactly: &, #, +, a space and a non-ASCII letter.
    const odd = 'a&b #c+é';
    await page.fill('input[name=q]', odd);
    await page.waitForFunction((t) => document.querySelector('#echo')?.textContent === t, odd, { timeout: 5000 }).catch(() => {});
    check('search: the exact text echoed', (await page.textContent('#echo')) === odd, await page.textContent('#echo'));

    // A slow older answer never lands after a newer one: the first /results is held 800 ms,
    // and the next request from the field aborts it.
    await fresh();
    await proxy('/_patterns/delay', 'POST');
    await page.fill('input[name=q]', 'mi');
    await page.waitForTimeout(400);
    await page.press('input[name=q]', 'End');
    await page.type('input[name=q]', 'l');
    await page.waitForTimeout(1500);
    seen = await loads();
    check('search: two requests, mi then mil', JSON.stringify(queries(seen)) === '["mi","mil"]', seen.gets);
    check('search: the newer answer stays', (await page.textContent('#echo')) === 'mil', await page.textContent('#echo'));

    // Infinite scroll: nothing before the sentinel shows; each scroll to the bottom loads the
    // next page once; the last page has no sentinel, so no further request.
    await fresh();
    await page.waitForTimeout(300);
    seen = await loads();
    check('scroll: nothing loaded above the fold', !seen.loads['/todos/more'], seen.loads);
    const toBottom = () => page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
    await toBottom();
    await page.waitForSelector('#row-20', { timeout: 5000 }).catch(() => {});
    await toBottom();
    await page.waitForSelector('#row-25', { timeout: 5000 }).catch(() => {});
    await toBottom();
    await page.waitForTimeout(500);
    seen = await loads();
    const mores = (seen.gets || []).filter((u) => u.startsWith('/todos/more')).map((u) => new URL(u, BASE).searchParams.get('after'));
    check('scroll: after=10 then after=20, once each', JSON.stringify(mores) === '["10","20"]', seen.gets);
    check('scroll: 25 rows', (await page.$$('li.row')).length === 25, (await page.$$('li.row')).length);

    await context.close();
  } finally {
    await browser.close();
  }
  process.stdout.write(JSON.stringify({ errors }));
})().catch((e) => {
  process.stderr.write(String((e && e.stack) || e));
  process.exit(1);
});
