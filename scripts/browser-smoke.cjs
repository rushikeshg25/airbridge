// Optional browser QA: npm install --no-save --package-lock=false playwright
// Build bin/airbridge first. Set CHROME_PATH if using system Chrome.
const { chromium } = require('playwright');
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const os = require('node:os');
const path = require('node:path');
const net = require('node:net');
const { spawn } = require('node:child_process');
const { randomBytes, createHash } = require('node:crypto');
const { once } = require('node:events');

const delay = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
async function freePort() {
  const server = net.createServer(); server.listen(0, '127.0.0.1');
  await once(server, 'listening'); const port = server.address().port;
  await new Promise((resolve) => server.close(resolve)); return port;
}
async function main() {
  const temporary = await fs.mkdtemp(path.join(os.tmpdir(), 'airbridge-browser-'));
  const port = await freePort(); const base = `http://127.0.0.1:${port}`;
  const pin = randomBytes(16).toString('hex');
  const screenshots = path.resolve('test-results'); await fs.mkdir(screenshots, { recursive: true });
  let host, browser;
  async function startHost() {
    host = spawn(path.resolve('bin/airbridge'), ['-listen', `127.0.0.1:${port}`, '-data', path.join(temporary, 'data')], {
      env: { ...process.env, AIRBRIDGE_PIN: pin }, stdio: ['ignore', 'ignore', 'pipe'],
    });
    let errors = ''; host.stderr.on('data', (data) => { errors += data; });
    for (let i = 0; i < 100; i++) {
      if (host.exitCode !== null) throw new Error(`Host exited: ${errors}`);
      try { if ((await fetch(base)).ok) return; } catch { /* wait for bind */ }
      await delay(50);
    }
    throw new Error('Host failed to start');
  }
  async function stopHost() {
    if (host && host.exitCode === null) { const stopped = once(host, 'exit'); host.kill('SIGTERM'); await stopped; }
  }
  async function join(page, name) {
    await page.goto(base);
    await page.locator('#join-dialog').waitFor({ state: 'visible' });
    await page.locator('#join-name').fill(name); await page.locator('#join-pin').fill(pin);
    await page.locator('#join-submit').click(); await page.locator('#join-dialog').waitFor({ state: 'hidden' });
  }
  try {
    await startHost();
    browser = await chromium.launch({ headless: true, executablePath: process.env.CHROME_PATH || undefined, args: ['--no-sandbox'] });
    const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, acceptDownloads: true });
    const sender = await context.newPage();
    const errors = []; sender.on('pageerror', (error) => errors.push(error.message));
    await join(sender, 'Desktop sender');
    await sender.screenshot({ path: path.join(screenshots, 'desktop.png'), fullPage: true });
    for (const width of [320, 390, 768, 1440]) {
      await sender.setViewportSize({ width, height: 900 });
      for (const view of ['send', 'library', 'help']) {
        await sender.locator(`.nav-item[data-view="${view}"]`).click();
        assert.equal(await sender.evaluate(() => document.documentElement.scrollWidth > innerWidth), false, `Overflow at ${width}, ${view}`);
      }
    }
    await sender.locator('.nav-item[data-view="send"]').click();
    const filePath = path.join(temporary, 'A large journey.bin');
    const payload = randomBytes(17 * 1024 * 1024 + 137); await fs.writeFile(filePath, payload);
    let reached; const atCheckpoint = new Promise((resolve) => { reached = resolve; });
    let release; const releaseResponse = new Promise((resolve) => { release = resolve; });
    let intercepted = false;
    await sender.route('**/api/files/*', async (route) => {
      if (route.request().method() === 'PATCH' && !intercepted) {
        intercepted = true;
        const response = await route.fetch(); reached(); await releaseResponse;
        try { await route.fulfill({ response }); } catch { /* The UI deliberately aborted this request. */ }
      } else await route.continue();
    });
    await sender.locator('#file-input').setInputFiles(filePath);
    await Promise.race([atCheckpoint, delay(15000).then(() => { throw new Error('Upload did not reach checkpoint'); })]);
    await sender.getByRole('button', { name: 'Pause', exact: true }).click(); release();
    await sender.locator('.transfer-row[data-status="paused"]').waitFor();
    const before = await sender.evaluate(async () => (await (await fetch('/api/files')).json()).files[0]);
    assert.equal(before.offset, 8 * 1024 * 1024);
    assert.equal(before.complete, false);
    await stopHost(); await startHost();
    await join(sender, 'Desktop sender');
    await sender.locator('#file-input').setInputFiles(filePath);
    await sender.locator('.transfer-row[data-status="done"]').waitFor({ timeout: 30000 });
    const after = await sender.evaluate(async () => (await (await fetch('/api/files')).json()).files[0]);
    assert.equal(after.id, before.id, 'Resume created a duplicate upload');
    assert.equal(after.size, payload.length);
    const smallPath = path.join(temporary, '<b>notes & résumé.txt');
    const small = Buffer.from('Unicode names and HTML-like names remain plain text.\n');
    await fs.writeFile(smallPath, small);
    const emptyPath = path.join(temporary, 'empty.txt'); await fs.writeFile(emptyPath, '');
    await sender.locator('#file-input').setInputFiles([smallPath, emptyPath]);
    await sender.waitForFunction(() => document.querySelectorAll('.transfer-row[data-status="done"]').length === 3);
    const receiverContext = await browser.newContext({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true, acceptDownloads: true });
    const receiver = await receiverContext.newPage(); receiver.on('pageerror', (error) => errors.push(error.message));
    await join(receiver, 'Phone receiver');
    await receiver.screenshot({ path: path.join(screenshots, 'mobile.png'), fullPage: true });
    await receiver.locator('.nav-item[data-view="library"]').click();
    await receiver.locator('.library-row').first().waitFor();
    await receiver.locator('#refresh-library').click();
    assert.equal(await receiver.locator('.library-row').count(), 3);
    assert.equal(await receiver.locator('.filename b').count(), 0, 'Filename interpreted as HTML');
    await receiver.screenshot({ path: path.join(screenshots, 'mobile-library.png'), fullPage: true });
    const downloadEvent = receiver.waitForEvent('download');
    await receiver.getByRole('link', { name: 'Download A large journey.bin', exact: true }).click();
    const download = await downloadEvent;
    const received = await fs.readFile(await download.path());
    assert.equal(createHash('sha256').update(received).digest('hex'), createHash('sha256').update(payload).digest('hex'));
    await receiver.locator('#file-search').fill('résumé');
    assert.equal(await receiver.locator('.library-row').count(), 1);
    await receiver.locator('#file-search').fill('not-found');
    await receiver.getByText('No files match your search.').waitFor();
    await receiver.locator('#file-search').fill('');
    receiver.once('dialog', (dialog) => dialog.accept());
    await receiver.getByRole('button', { name: 'Remove empty.txt from the room', exact: true }).click();
    await receiver.waitForFunction(() => document.querySelectorAll('.library-row').length === 2);
    await receiver.route('**/api/files', (route) => route.abort('connectionrefused'));
    await receiver.locator('#refresh-library').click();
    await receiver.locator('#offline-banner').waitFor({ state: 'visible' });
    await receiver.unroute('**/api/files');
    await receiver.locator('#refresh-library').click();
    await receiver.locator('#offline-banner').waitFor({ state: 'hidden' });
    // Hold fingerprinting while the user leaves. Neither the completed sample
    // nor the remaining selected files may enqueue work in the ended session.
    let sampled, releaseSample;
    const sampleReached = new Promise((resolve) => { sampled = resolve; });
    const sampleReleased = new Promise((resolve) => { releaseSample = resolve; });
    await receiver.route('**/api/fingerprint', async (route) => {
      const response = await route.fetch(); sampled(); await sampleReleased;
      await route.fulfill({ response });
    });
    await receiver.locator('.nav-item[data-view="send"]').click();
    await receiver.locator('#file-input').setInputFiles([smallPath, emptyPath]);
    await sampleReached;
    await receiver.locator('#leave-button').click();
    await receiver.locator('#join-dialog').waitFor({ state: 'visible' });
    releaseSample();
    await receiver.waitForResponse('**/api/fingerprint');
    await receiver.evaluate(() => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))));
    assert.equal(await receiver.locator('.transfer-row').count(), 0, 'Late sample enqueued files after logout');
    assert.equal(await receiver.evaluate(async () => (await fetch('/api/files')).status), 401);
    assert.deepEqual(errors, []);
    console.log('PASS: two browser sessions, 320/390/768/1440px layouts, pause, host restart, reselect-resume, 17 MiB SHA-256 round trip, empty/Unicode/HTML-like files, search, deletion, offline recovery, logout.');
  } finally {
    if (browser) await browser.close(); await stopHost();
    await fs.rm(temporary, { recursive: true, force: true });
  }
}
main().catch((error) => { console.error(error); process.exitCode = 1; });
