// smoke.cjs BASE: the boosted-navigation smoke test against the blog served at BASE, in a
// headless Chromium (Playwright). cmd/alder/browser_test.go runs it (go test -tags browser) and
// reads the JSON it prints: {"checks": [{"name", "ok", "got"}...]}. Every navigation here
// must happen without a document load: window.__m, set once, must survive them all.
const { chromium } = require('playwright');

const BASE = process.argv[2];
const DB = process.argv[3]; // the dev database file, to break a table in the 500 check
const checks = [];
const check = (name, ok, got) => checks.push({ name, ok: !!ok, got: JSON.stringify(got) });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

(async () => {
  const browser = await chromium.launch();
  try {
    const ctx = await browser.newContext({ viewport: { width: 800, height: 400 } });
    const page = await ctx.newPage();
    let docs = [];
    page.on('request', (r) => { if (r.resourceType() === 'document') docs.push(r.method() + ' ' + r.url()); });
    const state = () => page.evaluate(() => ({
      path: location.pathname, title: document.title, y: Math.round(scrollY), marker: window.__m === 1,
      h1: (document.querySelector('h1') || {}).textContent,
    }));
    const at = (path) => page.waitForFunction((p) => location.pathname === p, path, { timeout: 5000 }).catch(() => {});
    const link = (href) => page.evaluate((h) => {
      const a = document.createElement('a');
      a.id = 'go'; a.href = h; a.textContent = 'go';
      a.style.cssText = 'position:fixed;top:0;left:0;z-index:99;background:#fff';
      document.body.prepend(a);
    }, href);

    await page.goto(BASE + '/');
    await page.evaluate(() => { window.__m = 1; document.documentElement.style.minHeight = '4000px'; });
    await page.evaluate(() => scrollTo(0, 700));
    await sleep(400); // the scroll recorder's trailing call
    docs = [];

    // a boosted click: no document load; the URL, the title and the scroll change
    await link('/login');
    await page.click('#go');
    await at('/login');
    await sleep(200);
    let s = await state();
    check('boosted click: URL and title', s.path === '/login' && s.title === 'Log in', s);
    check('boosted click: no document load', docs.length === 0 && s.marker, docs);
    check('boosted click: scrolled to the top', s.y === 0, s.y);

    // Back and Forward each restore the scroll position the entry was left at
    await page.evaluate(() => scrollTo(0, 500));
    await sleep(400);
    await page.goBack({ waitUntil: 'commit' }).catch(() => {});
    await at('/');
    await page.waitForFunction(() => document.title === 'Blog', null, { timeout: 5000 }).catch(() => {});
    await sleep(300);
    s = await state();
    check('Back: the page and its scroll position', s.path === '/' && s.title === 'Blog' && Math.abs(s.y - 700) <= 2, s);
    await page.goForward({ waitUntil: 'commit' }).catch(() => {});
    await at('/login');
    await page.waitForFunction(() => document.title === 'Log in', null, { timeout: 5000 }).catch(() => {});
    await sleep(300);
    s = await state();
    check('Forward: the page and its scroll position', s.path === '/login' && s.title === 'Log in' && Math.abs(s.y - 500) <= 2, s);
    check('Back and Forward: no document load', docs.length === 0 && s.marker, docs);

    // the login form: its Redirect is morphed in, with the cookie
    await page.fill('input[name=email]', 'author@example.com');
    await page.fill('input[name=password]', 'password');
    await page.click('button[type=submit]');
    await at('/');
    await page.waitForFunction(() => document.title === 'Blog', null, { timeout: 5000 }).catch(() => {});
    await sleep(200);
    s = await state();
    const cookies = (await ctx.cookies()).map((c) => c.name);
    const out = await page.evaluate(() => document.body.innerText.includes('Log out'));
    check('login: redirect morphed in, signed in', s.path === '/' && s.title === 'Blog' && cookies.includes('alder_session') && out, { s, cookies, out });
    check('login: no document load', docs.length === 0 && s.marker, docs);

    // the dev panel: survives the morph, has its tabs, one trace
    const panel = await page.evaluate(() => {
      const p = document.querySelector('al-devpanel');
      return p && { tabs: [...p.querySelectorAll('nav button')].map((b) => b.textContent), bar: (p.querySelector('nav') || {}).textContent, trace: (document.querySelector('#_alder_tab_trace') || {}).textContent };
    });
    check('panel: present after a morph, with its tabs', panel && ['Request', 'Trace', 'SQL', 'Answer', 'Patches'].every((t) => panel.tabs.includes(t)), panel);
    check('panel: the bar names the request', panel && /POST|GET/.test(panel.bar), panel);
    await page.click('al-devpanel nav button:has-text("Trace")');
    const opened = await page.evaluate(() => { const p = document.querySelector('al-devpanel'); const o = p.hasAttribute('open'); p.removeAttribute('open'); return o; });
    check('panel: a tab opens it', opened, opened);
    check('panel: one trace', panel && /Db\.|Session\./.test(panel.trace), panel);

    // a link to a missing page: the 404 page morphed in, the URL recorded
    await link('/posts/no-such-post');
    await page.click('#go');
    await at('/posts/no-such-post');
    await page.waitForFunction(() => document.title === 'Not found', null, { timeout: 5000 }).catch(() => {});
    s = await state();
    check('404: morphed in', s.path === '/posts/no-such-post' && s.title === 'Not found' && s.h1 === 'Not found', s);
    check('404: no document load', docs.length === 0 && s.marker, docs);
    check('404: the panel answers not found', await page.evaluate(() => /not found/.test((document.querySelector('al-devpanel') || {}).textContent || '')));
    const r500 = await page.request.get(BASE + '/_dev');
    check('/_dev: the app map', r500.status() === 200 && (await r500.text()).includes('Routes'), r500.status());

    // a slow post, then a boosted click before its answer: the click's page stays — the two
    // share one AbortController (window._alderAC), so the post's late answer is never applied
    await link('/');
    await page.click('#go');
    await at('/');
    await page.waitForFunction(() => document.title === 'Blog', null, { timeout: 5000 }).catch(() => {});
    await page.route('**/_m/**', async (route) => {
      const resp = await route.fetch();
      await sleep(1500);
      await route.fulfill({ response: resp }).catch(() => {});
    });
    await page.click('text=Log out');
    await sleep(150);
    const busy = await page.evaluate(() => ({
      form: !!document.querySelector('form[aria-busy=true][inert]'),
      bar: document.documentElement.hasAttribute('data-alder-pending'),
    }));
    check('in flight: the form is inert and aria-busy, the pending bar shows', busy.form && busy.bar, busy);
    await link('/posts/no-such-post');
    await page.click('#go');
    await sleep(2500);
    s = await state();
    check('a slow post then a click: the click wins', s.path === '/posts/no-such-post' && s.title === 'Not found', s);
    check('a slow post then a click: no document load', docs.length === 0 && s.marker, docs);
    await page.unroute('**/_m/**');

    // Back during a pending boosted GET: the slow page never lands over the one Back returned to
    await link('/');
    await page.click('#go');
    await at('/');
    await sleep(300);
    await page.route('**/login*', async (route) => {
      const resp = await route.fetch();
      await sleep(1500);
      await route.fulfill({ response: resp }).catch(() => {});
    });
    await link('/login');
    await page.click('#go');
    await sleep(200);
    const pend = await page.evaluate(() => document.documentElement.hasAttribute('data-alder-pending'));
    await page.goBack();
    await sleep(2500);
    s = await state();
    check('Back during a pending GET: pending bar, then the earlier page', pend && s.path !== '/login', { pend, s, docs });
    await page.unroute('**/login*');

    // a link to a file under /static/ is left to the browser: a document load, not a morph
    await link('/static/x.pdf');
    await page.click('#go');
    await page.waitForURL('**/static/x.pdf', { timeout: 5000 }).catch(() => {});
    check('a link to a file: a plain navigation', docs.some((d) => d === 'GET ' + BASE + '/static/x.pdf'), docs);

    // ---- the interactive elements written in an Alder page (/smoke: testdata/browser/Smoke.alder,
    // added to the copy of the blog served here), as Appendix C writes them ----
    await page.goto(BASE + '/smoke');
    await page.waitForFunction(() => customElements.get('al-menu') && customElements.get('al-tooltip'), null, { timeout: 5000 }).catch(() => {});
    await page.click('#more-button');
    await sleep(150);
    const menu = await page.evaluate(() => {
      const m = document.getElementById('more');
      return { open: m.matches(':popover-open'), role: m.getAttribute('role'), items: m.querySelectorAll('[role=menuitem]').length, focus: document.activeElement.textContent };
    });
    check('menu (Alder page): <al-menu id popover> opens from its button, with roles and focus on the first item', menu.open && menu.role === 'menu' && menu.items === 2 && menu.focus === 'Home', menu);
    await page.keyboard.press('ArrowDown');
    const next = await page.evaluate(() => document.activeElement.textContent);
    await page.keyboard.press('Escape');
    await sleep(100);
    const menuShut = await page.evaluate(() => !document.getElementById('more').matches(':popover-open'));
    check('menu (Alder page): the arrow keys move between items, Escape closes it', next === 'Second' && menuShut, { next, menuShut });
    await page.click('#who-button');
    await sleep(100);
    const pop = await page.evaluate(() => document.getElementById('who').matches(':popover-open'));
    await page.keyboard.press('Escape');
    await sleep(100);
    const popShut = await page.evaluate(() => !document.getElementById('who').matches(':popover-open'));
    check('popover (Alder page): popovertarget opens it, Escape closes it', pop && popShut, { pop, popShut });
    const shown = () => page.evaluate(() => [...document.querySelectorAll('al-panel')].map((p) => p.checkVisibility()));
    const before = await shown();
    await page.click('al-tab label:has-text("Comments")');
    await sleep(100);
    const after = await shown();
    check('tabs (Alder page): the first panel shows, choosing a tab shows its panel only', before[0] && !before[1] && !after[0] && after[1], { before, after });
    await page.focus('#save-help');
    await sleep(150);
    const tip = await page.evaluate(() => {
      const t = document.querySelector('al-tooltip');
      return { open: t.matches(':popover-open'), described: document.getElementById('save-help').getAttribute('aria-describedby') === t.id };
    });
    check('tooltip (Alder page): focus on its trigger shows it, aria-describedby names it', tip.open && tip.described, tip);
    await page.click('#own-button');
    await sleep(150);
    const own = await page.evaluate(() => {
      const d = document.getElementById('own');
      return { open: d.open, modal: d.matches(':modal'), dialogs: document.querySelector('al-dialog').querySelectorAll('dialog').length };
    });
    check('dialog (Alder page): an author-written <dialog> inside <al-dialog> is one dialog, and opens as a modal', own.open && own.modal && own.dialogs === 1, own);
    await page.keyboard.press('Escape');

    // ---- the admin (M7b): the delete dialog across a morph, bulk toast, history, search, a 500 ----
    const ctx2 = await browser.newContext({ viewport: { width: 900, height: 700 } });
    const p2 = await ctx2.newPage();
    let docs2 = [];
    p2.on('request', (r) => { if (r.resourceType() === 'document') docs2.push(r.method() + ' ' + r.url()); });
    const at2 = (path) => p2.waitForFunction((x) => location.pathname === x, path, { timeout: 5000 }).catch(() => {});
    const link2 = (href) => p2.evaluate((h) => {
      const old = document.getElementById('go'); if (old) old.remove();
      const a = document.createElement('a'); a.id = 'go'; a.href = h; a.textContent = 'go'; document.body.prepend(a);
    }, href);
    // follow a link as a boosted click (a script click: a modal dialog makes the page inert to the mouse)
    const follow = async (href) => { await link2(href); await p2.evaluate(() => document.getElementById('go').click()); };
    await p2.goto(BASE + '/login');
    await p2.fill('input[name=email]', 'admin@example.com');
    await p2.fill('input[name=password]', 'password');
    await p2.click('button[type=submit]');
    await at2('/');
    await sleep(300);
    // 205 more users (a long reference table: more than 200 rows), one post
    for (let i = 0; i < 205; i++) {
      await p2.request.post(BASE + '/_m/Admin.User.New/Create', { form: { email: `u${i}@example.com`, name: `user ${i}`, role: 'Reader' } });
    }
    const newPage = await (await p2.request.get(BASE + '/admin/post/new')).text();
    const me = /<option value="([^"]+)">admin@example\.com/.exec(newPage)[1];
    await p2.request.post(BASE + '/_m/Admin.Post.New/Create', { form: { slug: 'hello', title: 'Hello', body: 'b', published: 'True', author: me } });
    await p2.goto(BASE + '/admin/post');
    const showHref = await p2.evaluate(() => [...document.querySelectorAll('a')].map((a) => a.getAttribute('href')).find((h) => /^\/admin\/post\/[^/]+$/.test(h) && !/new|bulk/.test(h)));
    await p2.goto(BASE + showHref);
    await p2.evaluate(() => { window.__m = 1; });
    docs2 = [];

    // the delete dialog opens as a modal, focus inside
    const dlg = () => p2.evaluate(() => {
      const d = document.querySelector('dialog');
      return d && { open: d.open, modal: d.matches(':modal'), inside: d.contains(document.activeElement), marker: window.__m === 1 };
    });
    await p2.click('button[command=show-modal]');
    await sleep(200);
    let d = await dlg();
    check('dialog: the Delete button opens a modal with focus inside', d && d.open && d.modal && d.inside, d);

    // RF1: a body morph while it is open leaves it open, modal, with focus inside
    await follow(showHref);
    await p2.waitForFunction(() => !document.getElementById('go'), null, { timeout: 5000 }).catch(() => {});
    await sleep(300);
    d = await dlg();
    check('dialog: a body morph leaves it open and modal, focus inside, no document load', d && d.open && d.modal && d.inside && d.marker && docs2.length === 0, { d, docs2 });

    // Escape closes it and gives focus back to the button
    await p2.keyboard.press('Escape');
    await sleep(200);
    const closed = await p2.evaluate(() => ({ open: document.querySelector('dialog').open, focus: document.activeElement.getAttribute('command') }));
    check('dialog: Escape closes it, focus returns to the button', !closed.open && closed.focus === 'show-modal', closed);

    // it shows the counts the confirmation page shows, a Delete button, and a link to that page
    await p2.click('button[command=show-modal]');
    await sleep(150);
    const inDlg = await p2.evaluate(() => {
      const d = document.querySelector('dialog');
      return { counts: [...d.querySelectorAll('dl dt')].map((t) => t.textContent), button: !!d.querySelector('form#delete button[type=submit]'), named: d.getAttribute('aria-labelledby') };
    });
    check('dialog: it shows the counts, the Delete button, and is named by its heading', inDlg.counts.length > 0 && inDlg.button && inDlg.named === 'delete-dialog-heading', inDlg);
    await p2.click('dialog a:has-text("See the details")');
    await at2(showHref + '/delete');
    await sleep(300);
    const s2 = await p2.evaluate(() => ({ path: location.pathname, h1: (document.querySelector('h1') || {}).textContent, marker: window.__m === 1 }));
    check('dialog: "See the details" leads to the confirmation page', s2.path === showHref + '/delete' && /Delete/.test(s2.h1) && s2.marker && docs2.length === 0, { s2, docs2 });

    // a 404 with the panel, then the panel's History and Copy as curl
    await follow('/posts/no-such-post');
    await at2('/posts/no-such-post');
    await sleep(400);
    const nf = await p2.evaluate(() => {
      const p = document.querySelector('al-devpanel');
      return p && { tabs: [...p.querySelectorAll(':scope > nav > button')].map((b) => b.textContent), text: p.textContent, sections: p.querySelectorAll(':scope > section').length };
    });
    check('404 with the panel: the answer is not found, History among the tabs', nf && /not found/.test(nf.text) && nf.tabs.includes('History'), nf);
    await p2.click('al-devpanel > nav > button:has-text("History")');
    await sleep(100);
    const hist = await p2.evaluate(() => {
      const items = [...document.querySelectorAll('#_alder_tab_history ol[data-hist] button')].map((b) => b.textContent);
      return { items, open: document.querySelector('al-devpanel').hasAttribute('open') };
    });
    check('history: the requests of this tab are listed, newest first, at most 20', hist.open && hist.items.length >= 3 && hist.items.length <= 20 && /\/posts\/no-such-post/.test(hist.items[0]), hist);
    await p2.click('#_alder_tab_history ol[data-hist] li:nth-child(3) button');
    const past = await p2.evaluate(() => (document.querySelector('#_alder_tab_history [data-past]') || {}).textContent || '');
    check('history: a click reopens a past request\'s sections', /Request/.test(past) && /Trace/.test(past) && /Patches/.test(past), past.slice(0, 200));
    await p2.click('al-devpanel > nav > button:has-text("Request")');
    const curlText = await p2.evaluate(() => (document.querySelector('#_alder_tab_request button[data-copy]') || {}).getAttribute('data-copy'));
    check('copy as curl: the line has the URL and the cookie placeholder', /^curl -X GET 'http:\/\/[^']*\/posts\/no-such-post'/.test(curlText || '') && /alder_session=<your session cookie>/.test(curlText), curlText);
    await p2.click('#_alder_tab_request button[data-copy]');
    await sleep(100);
    check('copy as curl: the button answers Copied', (await p2.evaluate(() => document.querySelector('#_alder_tab_request button[data-copy]').textContent)) === 'Copied');
    await p2.evaluate(() => document.querySelector('al-devpanel').removeAttribute('open'));
    await p2.reload();
    await sleep(300);
    const afterReload = await p2.evaluate(() => document.querySelectorAll('#_alder_tab_history ol[data-hist] button').length);
    check('history: it survives a full page load (sessionStorage)', afterReload >= 4, afterReload);

    // a toast from Notice.with: bulk delete of one chosen user, its notice on the list
    await p2.goto(BASE + '/admin/user?q=u17%40example.com');
    await p2.evaluate(() => { window.__m = 1; });
    await p2.check('input[name=ids]');
    await p2.click('button:has-text("Act on the chosen rows")');
    await p2.waitForFunction(() => location.pathname === '/admin/user/bulk', null, { timeout: 5000 }).catch(() => {});
    await sleep(300);
    // choose the panel's SQL tab, and close the panel: the tab stays chosen across the answer
    await p2.click('al-devpanel > nav > button:has-text("SQL")');
    await p2.evaluate(() => document.querySelector('al-devpanel').removeAttribute('open'));
    await p2.click('button:has-text("Delete the rows that can be deleted")');
    await p2.waitForFunction(() => location.pathname === '/admin/user', null, { timeout: 5000 }).catch(() => {});
    await p2.waitForSelector('al-toast', { timeout: 5000 }).catch(() => {});
    const toast = await p2.evaluate(() => {
      const t = document.querySelector('al-toast'), r = document.querySelector('al-toasts');
      return t && { text: t.textContent, shown: r.matches(':popover-open'), role: t.getAttribute('role') };
    });
    check('toast: Notice.with shows an al-toast in the open region', toast && /Deleted 1 of 1/.test(toast.text) && toast.shown && toast.role === 'status', toast);
    const hop = await p2.evaluate(() => {
      const p = document.querySelector('al-devpanel'), sql = document.getElementById('_alder_tab_sql').textContent;
      return {
        bar: p.querySelector(':scope > nav').textContent, request: document.getElementById('_alder_tab_request').textContent.slice(0, 60),
        del: sql.indexOf('DELETE'), then: sql.indexOf('then GET /admin/user'),
        shown: [...p.querySelectorAll(':scope > section')].filter((x) => !x.hidden).map((x) => x.id),
      };
    });
    check('panel after a redirect: the message\'s request, its DELETEs, then "then GET …" and the list\'s reads', /POST/.test(hop.request) && /redirect \/admin\/user, then page/.test(hop.bar) && hop.del >= 0 && hop.then > hop.del, hop);
    check('panel after an answer: one section shown, the tab chosen before', hop.shown.length === 1 && hop.shown[0] === '_alder_tab_sql', hop.shown);
    await p2.waitForFunction(() => !document.querySelector('al-toast'), null, { timeout: 8000 }).catch(() => {});
    check('toast: it times out', await p2.evaluate(() => !document.querySelector('al-toast')));

    // search: a long reference table (more than 200 rows) shows a search and 50 options
    await p2.goto(BASE + '/admin/post/new');
    const long = await p2.evaluate(() => ({ options: document.querySelectorAll('select[name=author] option').length, form: !!document.querySelector('form[action$="/SearchAuthor"]') }));
    check('search: more than 200 rows give 50 options and a search form', long.options === 50 && long.form, long);
    await p2.fill('form[action$="/SearchAuthor"] input[name=q]', 'u99@example.com');
    await p2.click('form[action$="/SearchAuthor"] button');
    await p2.waitForFunction(() => document.querySelectorAll('select[name=author] option').length === 1, null, { timeout: 5000 }).catch(() => {});
    const found = await p2.evaluate(() => ({ options: [...document.querySelectorAll('select[name=author] option')].map((o) => o.textContent), url: location.search }));
    check('search: it narrows the select (a Redirect, so the narrowing survives)', found.options.length === 1 && found.options[0] === 'u99@example.com' && /findAuthor/.test(found.url), found);
    // a crafted post of that user, with no search text: not among the 50 offered, so refused
    const u99 = await p2.evaluate(() => document.querySelector('select[name=author] option').value);
    const crafted = await p2.request.post(BASE + '/_m/Admin.Post.New/Create', { form: { slug: 'x', title: 'x', body: 'x', published: 'True', author: u99 } });
    check('search: an id outside the rendered options is refused', /must be one of the options/.test(await crafted.text()), crafted.status());

    // the Edit page of a post whose author sorts after the 50 offered: offered first, kept on save
    await p2.request.post(BASE + '/_m/Admin.User.New/Create', { form: { email: 'zz-last@example.com', name: 'last', role: 'Reader' } });
    await p2.goto(BASE + '/admin/user?q=zz-last');
    const zz = await p2.evaluate(() => { const c = document.querySelector('input[name=ids]'); return c && c.value; });
    // the New page offers that user only once the search finds it: post to the form's own action
    const newLate = await (await p2.request.get(BASE + '/admin/post/new?findAuthor=zz-last')).text();
    const createAt = /<form[^>]*action="([^"]*\/Create[^"]*)"/.exec(newLate)[1].replace(/&amp;/g, '&');
    await p2.request.post(BASE + createAt, { form: { slug: 'late', title: 'Late', body: 'b', published: 'True', author: zz } });
    await p2.goto(BASE + '/admin/post?q=late');
    const lateHref = await p2.evaluate(() => [...document.querySelectorAll('a')].map((a) => a.getAttribute('href')).find((h) => /^\/admin\/post\/[^/]+$/.test(h) && !/new|bulk/.test(h)));
    await p2.goto(BASE + lateHref + '/edit');
    const sel = await p2.evaluate(() => {
      const s = document.querySelector('select[name=author]');
      return { value: s.value, first: s.options[0].textContent, count: s.options.length };
    });
    check('edit past the 50 options: the row\'s own author is offered first and chosen', sel.value === zz && sel.first === 'zz-last@example.com' && sel.count === 51, sel);
    await p2.click('form[action*="/Save"] button[type=submit]');
    await at2(lateHref);
    await sleep(300);
    const kept = await p2.evaluate(() => [...document.querySelectorAll('#fields dd')].map((d) => d.textContent));
    check('edit past the 50 options: saving the form unchanged keeps the author', kept.includes('zz-last@example.com'), kept);

    // a refused message keeps the focus on the field (the form was inert while it was in flight):
    // the unique email of another user, which only the server can refuse
    await p2.goto(BASE + '/admin/user?q=zz-last');
    const zzEdit = (await p2.evaluate(() => [...document.querySelectorAll('a')].map((a) => a.getAttribute('href')).find((h) => /^\/admin\/user\/[^/]+$/.test(h) && !/new|bulk/.test(h)))) + '/edit';
    await p2.goto(BASE + zzEdit);
    await p2.route('**/_m/**', async (route) => { const resp = await route.fetch(); await sleep(400); await route.fulfill({ response: resp }).catch(() => {}); });
    await p2.fill('input[name=email]', 'admin@example.com');
    await p2.focus('input[name=email]');
    await p2.keyboard.press('Enter');
    await sleep(150);
    const during = await p2.evaluate(() => ({ inert: !!document.querySelector('form[inert]'), focus: document.activeElement && document.activeElement.tagName }));
    await p2.waitForFunction(() => !document.querySelector('form[inert]') && /with this email exists/.test(document.body.textContent), null, { timeout: 5000 }).catch(() => {});
    await sleep(200);
    const foc = await p2.evaluate(() => ({ name: document.activeElement && document.activeElement.getAttribute('name'), refused: /with this email exists/.test(document.body.textContent) }));
    await p2.unroute('**/_m/**');
    check('in flight: after a refused message the focus is back on the field', during.inert && foc.refused && foc.name === 'email', { during, foc });

    // the dialog's own Remove: it deletes the row from the Show page
    await p2.goto(BASE + lateHref);
    await p2.click('button[command=show-modal]');
    await sleep(150);
    await p2.click('dialog form#delete button[type=submit]');
    await at2('/admin/post');
    await sleep(300);
    const gone = await p2.request.get(BASE + lateHref);
    check('dialog: its Delete posts the Show page\'s Remove, and the row is gone', (await p2.evaluate(() => location.pathname)) === '/admin/post' && gone.status() === 404, gone.status());

    // RF2: a 500 with the panel: break a table in the dev database, follow a link to its list
    if (DB) {
      const sql = (q) => {
        try { const { DatabaseSync } = require('node:sqlite'); const db = new DatabaseSync(DB); db.exec(q); db.close(); return true; } catch (e) {
          try { require('child_process').execFileSync('sqlite3', [DB, q]); return true; } catch (e2) { return false; }
        }
      };
      await p2.goto(BASE + '/admin');
      await p2.evaluate(() => { window.__m = 1; });
      docs2 = [];
      const broke = sql('ALTER TABLE Comment__Comment RENAME TO Comment__Hidden');
      await follow('/admin/comment');
      await at2('/admin/comment');
      await sleep(700);
      const e500 = await p2.evaluate(() => {
        const p = document.querySelector('al-devpanel');
        return p && { text: p.textContent, marker: window.__m === 1 };
      });
      check('500 with the panel: morphed in, the panel shows an error answer with its text', broke && e500 && e500.marker && /error/.test(e500.text) && /no such table|Comment/.test(e500.text) && docs2.length === 0, { broke, e500: e500 && e500.text.slice(0, 160), docs2 });
      await p2.click('al-devpanel > nav > button:has-text("Answer")');
      check('500 with the panel: its tabs still work', await p2.evaluate(() => document.querySelector('al-devpanel').hasAttribute('open')));
      sql('ALTER TABLE Comment__Hidden RENAME TO Comment__Comment');
      await p2.evaluate(() => document.querySelector('al-devpanel').removeAttribute('open'));
      await follow('/admin/comment');
      await sleep(600);
      check('500 with the panel: the next request is served again', await p2.evaluate(() => /Comments/.test(document.title + (document.querySelector('h1') || {}).textContent)));
    } else {
      check('500 with the panel: skipped (no database path)', true);
    }
  } finally {
    await browser.close();
  }
  console.log(JSON.stringify({ checks }));
})().catch((e) => { console.error(e && e.stack || e); process.exit(1); });
