// routedelete.cjs BASE: sends each request of the page at BASE that targets a DELETE route
// (#160), first with JavaScript (Datastar) and then without, and waits for each to be answered;
// the server records them. cmd/alder/routes_browser_test.go (TestBrowser_RouteDeleteBody) runs
// it, reads the JSON it prints ({"errors": [page errors]}) and checks what the server recorded.
const { chromium } = require('playwright');

const [BASE] = process.argv.slice(2);
const errors = [];

// act runs step on page and waits for the answer to the request to path.
async function act(page, path, step) {
  const answered = page.waitForResponse((r) => new URL(r.url()).pathname === path, { timeout: 5000 });
  await step();
  await answered;
}

(async () => {
  const browser = await chromium.launch();
  try {
    for (const js of [true, false]) {
      const context = await browser.newContext({ javaScriptEnabled: js });
      const page = await context.newPage();
      page.on('pageerror', (e) => errors.push(String(e)));
      const fresh = async () => {
        await page.goto(BASE + '/');
        if (js) await page.waitForFunction(() => typeof window._alderO === 'function');
      };
      await fresh();
      await act(page, '/d/1', () => page.click('#b1'));
      if (!js) await fresh();
      await act(page, '/d/2', () => page.click('#b2'));
      if (!js) await fresh();
      await act(page, '/d/3', () => page.press('#i3', 'Enter'));
      if (!js) await fresh();
      await act(page, '/d/4', () => page.click('#b4'));
      if (!js) await fresh();
      await act(page, '/d/6', () => page.press('#i6', 'Enter'));
      if (js) {
        await act(page, '/d/5', () => page.click('#b5'));
      }
      await page.waitForTimeout(200);
      await context.close();
    }
  } finally {
    await browser.close();
  }
  process.stdout.write(JSON.stringify({ errors }));
})().catch((e) => {
  process.stderr.write(String(e && e.stack || e));
  process.exit(1);
});
