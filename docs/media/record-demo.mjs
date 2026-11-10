// SPDX-License-Identifier: MIT
//
// Makes the README's screenshots (docs/images), the demo video and its
// subtitles (docs/media), and the illustrations of docs/PLAYBOOKS.md. It drives
// the web UI of a running iec61850-explorerd, in the local Chrome, through the
// demo substation AA1 Northfield (demo/README.md): its SCL files, its vendor
// namespace, its captures and its five IEDs served live by "make demo-ieds".
//
//   make build-daemon
//   make demo-ieds                                     # in another terminal: the IEDs on 127.0.0.1:10201-10205
//   D=$(mktemp -d)                                     # an empty data directory for each run (mode 0700)
//   bin/iec61850-explorerd --listen 127.0.0.1:18790 --data-dir $D --allow-target 127.0.0.0/8 --allow-commission
//   bin/iec61850-explorerd url --data-dir $D > $SCRATCH/url.txt
//   cp docs/media/record-demo.mjs $SCRATCH && cd $SCRATCH && npm i playwright   # not in the repo
//   node record-demo.mjs shots out/png url.txt $REPO    # 2x screenshots (dark, one light) -> copy to docs/images
//   node record-demo.mjs video out/vid url.txt $REPO    # frames, demo.vtt, demo.mp4 (soft subtitles), demo.gif
//   pngquant --skip-if-larger --ext .png --force out/png/*.png   # optional, before copying the screenshots
//
// Video mode runs ffmpeg itself (it needs only libx264, no libass): the
// captions are drawn into the page, so they are in every frame of the MP4 and
// the GIF, and the same cues are written as WebVTT and muxed into the MP4 as a
// soft subtitle track (mov_text, eng). Cue times are taken from the recorded
// frames, not estimated.
//
// Restart "make demo-ieds" and the daemon with a new data directory (and write
// url.txt again) before each run: the demo opens feeder 1's breaker QA1 (the
// interlock refusal needs it closed), and the vendor NSD package it loads stays
// in the data directory.
import { chromium } from 'playwright';
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import { join, resolve } from 'node:path';

const mode = process.argv[2] ?? 'shots';
const out = resolve(process.argv[3] ?? `out/${mode}`);
const url = readFileSync(process.argv[4] ?? 'url.txt', 'utf8').trim();
const repo = resolve(process.argv[5] ?? '.');
const demo = (p) => join(repo, 'demo', p);
const LIVE = { host: '127.0.0.1', port: '10201' }; // feeder 1, AA1D1Q02A1
mkdirSync(join(out, 'downloads'), { recursive: true });

const video = mode === 'video';
const browser = await chromium.launch({ channel: 'chrome', headless: true });
const context = await browser.newContext({
  viewport: video ? { width: 1280, height: 760 } : { width: 1440, height: 860 },
  deviceScaleFactor: video ? 1 : 2,
  colorScheme: 'dark',
  acceptDownloads: true,
});
if (video) {
  // A visible pointer (recorded videos have none) and the caption box, in the
  // OT Fabric tokens of the page, so it reads on both themes.
  await context.addInitScript(() => {
    addEventListener('DOMContentLoaded', () => {
      const dot = document.createElement('div');
      dot.style.cssText =
        'position:fixed;z-index:2147483647;width:18px;height:18px;margin:-9px 0 0 -9px;border-radius:50%;' +
        'background:rgba(63,193,198,.35);border:2px solid #3fc1c6;pointer-events:none;transition:transform .08s;left:-40px;top:-40px';
      document.body.appendChild(dot);
      addEventListener('mousemove', (e) => { dot.style.left = e.clientX + 'px'; dot.style.top = e.clientY + 'px'; }, true);
      addEventListener('mousedown', () => { dot.style.transform = 'scale(.7)'; }, true);
      addEventListener('mouseup', () => { dot.style.transform = ''; }, true);
      const cap = document.createElement('div');
      cap.id = 'demo-caption';
      cap.style.cssText =
        'position:fixed;z-index:2147483646;left:50%;bottom:28px;transform:translateX(-50%);max-width:920px;width:max-content;' +
        'padding:10px 18px;border-radius:6px;border:1px solid var(--ot-accent);' +
        'background:color-mix(in srgb, var(--ot-surface) 94%, transparent);color:var(--ot-fg-strong);' +
        'box-shadow:0 6px 24px rgba(0,0,0,.35);font:500 17px/1.45 var(--ot-font-body);text-align:center;' +
        'pointer-events:none;opacity:0;transition:opacity .3s';
      document.body.appendChild(cap);
    });
  });
}
const page = await context.newPage();
page.on('download', (d) => void d.saveAs(join(out, 'downloads', d.suggestedFilename())).catch(() => {}));

// Video: Chrome's screencast frames with their timestamps; ffmpeg assembles them.
const frames = [];
let cdp;
if (video) {
  cdp = await context.newCDPSession(page);
  cdp.on('Page.screencastFrame', async (f) => {
    frames.push({ t: f.metadata.timestamp, data: f.data });
    await cdp.send('Page.screencastFrameAck', { sessionId: f.sessionId }).catch(() => {});
  });
}
const pause = (ms) => page.waitForTimeout(video ? ms : Math.min(ms, 600));
const shot = async (name) => {
  if (video) return;
  await page.mouse.move(2, 2);
  await page.waitForTimeout(700);
  await page.screenshot({ path: join(out, `${name}.png`) });
  console.log('shot', name);
};
let failures = 0;
const step = async (name, fn) => {
  try {
    await fn();
  } catch (e) {
    failures++;
    console.log('STEP FAILED', name, String(e).split('\n')[0]);
    await page.screenshot({ path: join(out, `failed-${name.replace(/ /g, '-')}.png`) }).catch(() => {});
  }
};

// Captions: one cue list, shown in the page (video only) and written as WebVTT.
// A cue lasts from its caption until the next one (or until it is hidden);
// its times come from the screencast frames' clock.
const cues = [];
const now = async () => (video ? (await cdp.send('Runtime.evaluate', { expression: 'Date.now()', returnByValue: true })).result.value / 1000 : 0);
const caption = async (html) => {
  if (!video) return;
  const t = await now();
  const last = cues.at(-1);
  if (last && last.end === undefined) last.end = t;
  await page.evaluate((h) => {
    const cap = document.getElementById('demo-caption');
    if (!cap) return;
    if (h) cap.innerHTML = h;
    cap.style.opacity = h ? '1' : '0';
  }, html);
  if (html) cues.push({ start: t, html });
};
// The caption sits at the bottom, or at the top while the bottom is what is shown.
const captionAt = async (where) => {
  if (!video) return;
  await page.evaluate((w) => {
    const cap = document.getElementById('demo-caption');
    if (!cap) return;
    cap.style.bottom = w === 'top' ? '' : '28px';
    cap.style.top = w === 'top' ? '6px' : '';
  }, where);
};
// Video sections for the GIF highlight cut.
const sections = [];
const highlight = async (seconds) => { if (video) sections.push({ start: await now(), seconds }); };

// Moves the pointer to an element and clicks it, visibly; the caption moves
// out of the way when it would cover the element.
const click = async (locator, opts = {}) => {
  await locator.scrollIntoViewIfNeeded();
  const box = await locator.boundingBox();
  if (box) {
    if (video) {
      const cap = await page.evaluate(() => document.getElementById('demo-caption')?.getBoundingClientRect().toJSON());
      if (cap && box.y + box.height > cap.top - 8 && box.y < cap.bottom + 8) {
        await captionAt(cap.top > 300 ? 'top' : 'bottom');
      }
    }
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2, { steps: video ? 14 : 1 });
    await pause(180);
  }
  await locator.click(opts);
};
const typeInto = async (locator, text) => {
  await click(locator);
  await locator.fill('');
  await locator.pressSequentially(text, { delay: video ? 35 : 0 });
};
const selectIn = async (select, option) => {
  await click(select);
  await select.selectOption(option);
};
// A file is chosen through the button that opens the file dialog, as a user would.
const chooseFile = async (button, path) => {
  const [chooser] = await Promise.all([page.waitForEvent('filechooser'), click(button)]);
  await chooser.setFiles(path);
};

const tab = (name) => page.getByRole('tab', typeof name === 'string' ? { name, exact: true } : { name });
const sessionTab = (text) => page.getByRole('navigation', { name: 'Sessions' }).getByRole('button', { name: text });
const newTab = () => page.getByRole('navigation', { name: 'Sessions' }).getByRole('button', { name: 'New' });
const bar = (name) => page.getByRole('button', { name, exact: true });
const treeItem = (name) => page.getByRole('treeitem').filter({ has: page.getByText(name, { exact: true }) }).first();
const expand = async (name) => {
  const b = page.getByRole('button', { name: `Expand ${name}`, exact: true });
  if (await b.count()) await click(b);
};
const selectNode = async (name) => click(treeItem(name).getByText(name, { exact: true }));
const search = async ({ text = '', quantity = '' }) => {
  const box = page.getByRole('search');
  await typeInto(box.getByLabel('Search references'), text);
  await selectIn(box.getByLabel('Quantity'), quantity);
  await click(box.getByRole('button', { name: 'Search' }));
  await page.getByRole('list', { name: 'Search results' }).waitFor({ timeout: 15000 });
};
const pickHit = async (re) => click(page.getByRole('list', { name: 'Search results' }).getByRole('button').filter({ hasText: re }).first());
const openSCL = async (file, ied) => {
  await chooseFile(page.getByRole('button', { name: 'Open SCL file…' }), demo(`scl/${file}`));
  const dialog = page.getByRole('dialog', { name: 'SCL file' });
  await dialog.waitFor({ timeout: 15000 });
  await pause(1000);
  await click(dialog.getByRole('listitem').filter({ hasText: ied }).getByRole('button', { name: 'Browse offline' }));
  await page.getByRole('tree').getByRole('treeitem').first().waitFor({ timeout: 15000 });
};
const openCapture = async (file) => {
  await chooseFile(page.getByRole('button', { name: 'Open capture…' }), demo(`captures/${file}`));
  await page.getByRole('region', { name: 'Trace summary' }).waitFor({ timeout: 60000 });
};
const link = async (fileLabel) => {
  const sel = page.getByLabel('Correlate with model');
  const value = await sel.locator('option').filter({ hasText: fileLabel }).first().getAttribute('value');
  await selectIn(sel, value);
  await click(page.getByRole('button', { name: 'Link', exact: true }));
  await page.getByText(/^Linked to /).waitFor({ timeout: 30000 });
};
const fcBlock = (fc) => page.getByRole('region', { name: 'Selected node' }).locator('div.rounded').filter({ has: page.locator('span.font-mono', { hasText: new RegExp(`^${fc}$`) }) }).first();

await page.goto(url);
await page.getByRole('button', { name: 'Open SCL file…' }).waitFor({ timeout: 30000 });
await page.waitForTimeout(1500);
if (await page.getByText('No valid token').count()) {
  console.log('the daemon rejected the token: write url.txt again ("iec61850-explorerd url") after the daemon has started');
  process.exit(2);
}
if (video) {
  await cdp.send('Page.startScreencast', { format: 'png', maxWidth: 1280, maxHeight: 760, everyNthFrame: 1 });
  await page.waitForTimeout(300);
}
const t0 = await now();

// 1. The start view.
await highlight(6);
await caption('IEC 61850 Explorer: connect to an IED, open an SCL file, or analyse a capture.<br>The demo: <b>AA1 Northfield</b>, a made-up substation in the repository.');
await pause(7500);
await shot('start');

// 2. The SCD, offline.
await step('open SCD', async () => {
  await caption('Open the substation’s SCD. It lists six IEDs; browse the transformer protection offline, no IED needed.');
  await pause(2000);
  await openSCL('AA1_Northfield.scd', 'AA1E1Q01A1');
  await pause(1200);
});
await step('browse', async () => {
  await caption('Its model as the SCL defines it: logical devices, logical nodes, data objects and their functional constraints.');
  await expand('AA1E1Q01A1MEAS');
  await pause(700);
  await expand('MMXU2');
  await pause(900);
  await selectNode('TotW');
  await pause(2200);
});
await step('search impedance', async () => {
  await highlight(10);
  await caption('Search by quantity: the IEC 61850 namespace knows which data objects measure impedance, energy, voltage…');
  await search({ quantity: 'impedance' });
  await pause(2700);
  await pickHit(/MMXU1\.Z \[/);
  await pause(1200);
  await caption('The namespace panel says what the selected node is: class, CDC, presence, and the quantity, phase and unit.');
  await pause(5000);
});
if (!video) await step('search energy', async () => {
  await search({ quantity: 'active-energy' });
  await pause(800);
  await pickHit(/MMTR1\.SupWh \[/);
  await caption('<i>Active energy</i> finds the meter’s total, supply and demand registers.');
  await pause(4000);
});
await shot('scl-search');

// 3. Conformance: the vendor namespace, then Edition 1.
await step('check vendor', async () => {
  await click(bar('Check'));
  await page.getByRole('region', { name: 'Conformance' }).waitFor();
  await click(page.getByRole('region', { name: 'Conformance' }).getByRole('button', { name: 'Check', exact: true }));
  await page.getByRole('list', { name: 'Declared namespaces' }).waitFor({ timeout: 15000 });
  await caption('Check conformance to the namespace. YLTC1 declares the vendor namespace <code>CORVANE 7-4X:2026A</code>, which is not loaded.');
  await pause(1000);
  await page.getByRole('list', { name: 'Declared namespaces' }).getByText('not loaded').first().evaluate((el) => el.scrollIntoView({ block: 'start', behavior: 'smooth' }));
  await pause(4500);
});
await shot('conformance-vendor');
await step('load NSD', async () => {
  const region = page.getByRole('region', { name: 'Conformance' });
  await region.evaluate((el) => el.scrollTo({ top: 0, behavior: 'smooth' }));
  await caption('Load the vendor’s NSD package and check again: its logical node and data objects are now checked against their definitions.');
  await chooseFile(region.getByRole('button', { name: 'Load NSD package…' }), demo('nsd/corvane-7-4x-2026a.nsd'));
  await region.getByRole('list', { name: 'Loaded packages' }).waitFor({ timeout: 15000 });
  await pause(800);
  await click(region.getByRole('button', { name: 'Check', exact: true }));
  await region.getByRole('list', { name: 'Declared namespaces' }).waitFor({ timeout: 15000 });
  await pause(4500);
});
await step('edition 1', async () => {
  await click(newTab());
  await caption('Feeder 2’s CID is an Edition 1 file: its logical devices declare no namespace, so they are checked as Edition 1.');
  await openSCL('AA1D1Q03A1.cid', 'AA1D1Q03A1');
  await click(bar('Check'));
  const region = page.getByRole('region', { name: 'Conformance' });
  await click(region.getByRole('button', { name: 'Check', exact: true }));
  await region.getByRole('list', { name: 'Declared namespaces' }).waitFor({ timeout: 15000 });
  await pause(5000);
});
await shot('conformance-edition1');
await step('points', async () => {
  await caption('Export the point list for SCADA or gateway engineering: every attribute with FC, CDC, type, quantity, data sets and reports.');
  await click(page.getByRole('button', { name: 'Points CSV' }));
  await pause(5000);
});

// 4. Live: feeder 1 on 127.0.0.1:10201.
await step('feeder 1 CID', async () => {
  await click(newTab());
  await caption('Now live. First open feeder 1’s CID, to compare with the IED later.');
  await pause(1500);
  await openSCL('AA1D1Q02A1.cid', 'AA1D1Q02A1');
  await pause(400);
});
await step('connect', async () => {
  await click(newTab());
  await captionAt('bottom');
  await caption('Connect to feeder 1, served by <code>make demo-ieds</code> on 127.0.0.1:10201. The session starts in Observe mode: reads only.');
  const form = page.getByRole('region', { name: 'Connect to an IED' });
  await typeInto(form.getByLabel('IED host'), LIVE.host);
  await typeInto(form.getByLabel('Port'), LIVE.port);
  await click(form.getByRole('button', { name: 'Connect' }));
  await page.getByRole('tree').getByRole('treeitem').first().waitFor({ timeout: 30000 });
  await pause(2500);
});
await step('read', async () => {
  await search({ text: 'MMXU1.TotW', quantity: '' });
  await pickHit(/MMXU1\.TotW \[/);
  await pause(600);
  await click(fcBlock('MX').getByRole('button', { name: 'Read' }));
  await caption('Read a data object: the value with its quality and time stamp, and the MMS domain and item it was read from.');
  await pause(4500);
});
await step('watch', async () => {
  await click(fcBlock('MX').getByRole('button', { name: 'Watch' }));
  await search({ text: 'MMXU1.Hz', quantity: '' });
  await pickHit(/MMXU1\.Hz \[/);
  await click(fcBlock('MX').getByRole('button', { name: 'Watch' }));
  await click(fcBlock('MX').getByRole('button', { name: 'Read' }));
  await captionAt('top');
  await caption('Watch points of any IED in one table and poll them; the measurements change every 2 s.');
  await selectIn(page.getByLabel('Poll interval'), '1000');
  await click(page.getByRole('button', { name: 'Poll', exact: true }));
  await pause(5000);
});
await shot('live-watch');
await step('data sets', async () => {
  await captionAt('bottom');
  await click(page.getByRole('search').getByRole('button', { name: 'Clear' }).or(page.getByRole('button', { name: 'Clear' })).first());
  await selectNode('AA1D1Q02A1MEAS');
  await page.getByRole('region', { name: 'Logical device' }).waitFor();
  await caption('A logical device shows its data sets, report control blocks, logs and setting groups. Read a data set in one request.');
  await click(page.getByRole('region', { name: 'Logical device' }).getByRole('button', { name: 'Read', exact: true }).first());
  await pause(4500);
});
await shot('datasets');
if (!video) await step('rcbs', async () => {
  await click(tab('Report control blocks'));
  await caption('Report control blocks with their configuration as read from the IED.');
  await pause(3000);
});
await step('setting groups', async () => {
  await selectNode('AA1D1Q02A1PROT');
  await page.getByRole('region', { name: 'Logical device' }).waitFor();
  await click(tab('Setting groups'));
  await caption('Setting groups: the protection has two, group 1 is active.');
  await pause(3000);
});
await step('files', async () => {
  await click(bar('Files'));
  await page.getByRole('region', { name: 'Files' }).getByRole('table').waitFor({ timeout: 15000 });
  await caption('The IED’s files: its CID and the COMTRADE record of this morning’s fault, ready to download.');
  await pause(1000);
  await click(page.getByRole('button', { name: /^Download COMTRADE\/.*\.cfg$/ }));
  await pause(3500);
});
await shot('files');
await step('compare', async () => {
  await highlight(9);
  await click(bar('Compare'));
  const region = page.getByRole('region', { name: 'Compare' });
  const ref = region.getByLabel('A: reference');
  const value = await ref.locator('option').filter({ hasText: 'AA1D1Q02A1.cid' }).first().getAttribute('value');
  await selectIn(ref, value);
  await click(region.getByLabel('Values'));
  await caption('Compare the IED with its CID, configured values included (FC CF, DC, SP, SG).');
  await pause(2500);
  await click(region.getByRole('button', { name: 'Compare', exact: true }));
  await region.getByRole('list', { name: 'Summary' }).waitFor({ timeout: 60000 });
  await pause(2000);
  await caption('Exactly two differences: someone updated the firmware (2.4.1) and set I> to 650 A on the front panel.');
  await pause(5500);
});
await shot('compare');

// 5. Commission mode: a grant, a report subscription, controls, the audit.
await step('grant', async () => {
  await click(bar('Model'));
  await click(bar('Commission…'));
  const dialog = page.getByRole('dialog', { name: 'Commission mode' });
  await dialog.waitFor();
  await caption('Changes need Commission mode: a grant with a reason, a duration and capabilities, on a second, restricted association.');
  await typeInto(dialog.getByLabel(/^Reason/), 'WO-1110 isolate feeder 1');
  await click(dialog.getByLabel(/^Subscribe to report control blocks/));
  await click(dialog.getByLabel(/^Operate controls/));
  await pause(800);
  await click(dialog.getByRole('button', { name: 'Grant' }));
  await page.getByRole('status', { name: 'Commission grant' }).waitFor({ timeout: 15000 });
  await pause(1500);
});
await step('subscribe', async () => {
  await selectNode('AA1D1Q02A1MEAS');
  await click(tab('Report control blocks'));
  await click(page.getByRole('region', { name: 'Logical device' }).getByRole('button', { name: 'Subscribe' }).first());
  await click(tab(/^Reports \(/));
  await captionAt('top');
  await caption('Subscribe to a report control block: the reports arrive with their reasons for inclusion.');
  await page.getByRole('region', { name: 'Reports' }).getByRole('listitem').first().waitFor({ timeout: 15000 });
  await pause(4500);
});
await shot('reports');
await step('interlock', async () => {
  await highlight(14);
  await captionAt('bottom');
  await search({ text: 'CSWI2.Pos', quantity: '' });
  await pickHit(/CSWI2\.Pos \[/);
  const form = page.getByRole('form', { name: 'Control' });
  await form.scrollIntoViewIfNeeded();
  await caption('Open disconnector QB1 (SBO, enhanced security). You confirm before anything is sent.');
  await pause(2000);
  await typeInto(form.getByLabel('ctlVal'), 'false');
  await click(form.getByRole('button', { name: 'Operate…' }));
  await pause(1000);
  await click(form.getByRole('group', { name: 'Confirm the control' }).getByRole('button', { name: 'Operate' }));
  await form.getByLabel('Control outcome').waitFor({ timeout: 30000 });
  await caption('The IED refuses: <i>blocked-by-interlocking</i>, because breaker QA1 is still closed.');
  await pause(4500);
});
await shot('control-refused');
await step('operate', async () => {
  await search({ text: 'CSWI1.Pos', quantity: '' });
  await pickHit(/CSWI1\.Pos \[/);
  const form = page.getByRole('form', { name: 'Control' });
  await form.scrollIntoViewIfNeeded();
  await caption('Open breaker QA1 first: select, operate and the CommandTermination, each step shown.');
  await typeInto(form.getByLabel('ctlVal'), 'false');
  await click(form.getByRole('button', { name: 'Operate…' }));
  await pause(1000);
  await click(form.getByRole('group', { name: 'Confirm the control' }).getByRole('button', { name: 'Operate' }));
  await form.getByLabel('Control outcome').waitFor({ timeout: 30000 });
  await pause(4000);
});
await shot('control-ok');
await step('audit', async () => {
  await click(tab(/^Audit \(/));
  await captionAt('top');
  await caption('The audit trail records the grant, the subscription and every control, refused or not.<br><i>Revoke</i> ends the grant and releases the report control block.');
  await pause(5500);
});
await shot('audit');
await step('revoke', async () => {
  await click(page.getByRole('status', { name: 'Commission grant' }).getByRole('button', { name: 'Revoke' }));
  await pause(1500);
  await captionAt('bottom');
});

// 6. Capture 07, the station fault.
await step('open capture 07', async () => {
  await click(tab(/^Watch \(/));
  await click(page.getByRole('button', { name: 'Stop polling' }));
  await click(newTab());
  await caption('Analyse a capture offline: the station fault at 09:00:10, with MMS, GOOSE and Sampled Values on one mirror port.');
  await openCapture('07-station-fault.pcapng');
  await pause(5000);
});
await step('link 07', async () => {
  await caption('Link it to feeder 1’s SCL model: MMS names become IEC 61850 references, and reports are decoded against their data sets.');
  await link('AA1D1Q02A1.cid');
  await pause(5500);
});
await step('timeline 07', async () => {
  await click(tab('timeline'));
  await page.getByRole('region', { name: 'Timeline' }).getByRole('table').waitFor();
  await caption('The timeline: every confirmed service with its status, latency and the objects it named.');
  await pause(1000);
  await click(page.getByTestId('transaction').nth(3));
  await pause(3500);
});
await shot('trace-timeline');
await step('reports 07', async () => {
  await highlight(9);
  await click(tab('reports'));
  await page.getByRole('region', { name: 'Captured reports' }).waitFor();
  await caption('Feeder 1’s reports, decoded against the data set <code>dsTrip</code>: the starts, the operate and the trip, each with its reason.');
  await pause(800);
  await page.getByRole('region', { name: 'Captured reports' }).getByRole('listitem').filter({ hasText: '09:00:10.012' }).first()
    .evaluate((el) => el.scrollIntoView({ block: 'start', behavior: 'smooth' }));
  await pause(4500);
});
await shot('trace-reports');
await step('streams 07', async () => {
  await click(tab('streams'));
  await page.getByRole('region', { name: 'Streams' }).waitFor();
  await caption('GOOSE: feeder 1’s trip, then the breaker failure 152 ms later. Sampled Values: 6000 frames, none lost.');
  await pause(2000);
  await page.getByRole('region', { name: 'Streams' }).getByText('Sampled Values', { exact: true }).evaluate((el) => el.scrollIntoView({ block: 'center', behavior: 'smooth' }));
  await pause(3500);
});
await shot('trace-streams');

// 7. Capture 08: the problems.
await step('open capture 08', async () => {
  await highlight(8);
  await click(newTab());
  await caption('Capture 08: a bad morning on the station bus. Link it to feeder 2 and open the findings.');
  await pause(1500);
  await openCapture('08-problems.pcapng');
  await link('AA1D1Q03A1.cid');
  await click(tab('findings'));
  await page.getByRole('region', { name: 'Findings' }).getByRole('table').waitFor();
  await pause(2000);
  await caption('A lost TCP segment, unanswered requests, a midstream association, a reject, GOOSE and SV losses: each with where and why.');
  await pause(4500);
  await selectIn(page.getByRole('region', { name: 'Findings' }).getByLabel('Severity'), 'error');
  await pause(2500);
});
await shot('trace-findings');
if (!video) await step('export findings', async () => click(page.getByRole('button', { name: 'Findings CSV' })));

// The end card; screenshots only: the light theme.
if (!video) {
  await step('light', async () => {
    await sessionTab(/^AA1D1Q02A1 \(AA1D1Q02A1\.cid\)/).click().catch(() => {});
    await page.getByRole('button', { name: 'Switch to light theme' }).click();
    await page.waitForTimeout(800);
    await sessionTab(/^07-station-fault/).click();
    await tab('streams').click();
    await shot('light');
    await page.getByRole('button', { name: 'Switch to dark theme' }).click();
  });
}
await step('end', async () => {
  await click(newTab());
  await caption('IEC 61850 Explorer by OT Fabric: inspect, commission, compare and analyse. MIT licensed.');
  await pause(4500);
  await caption('');
  await pause(600);
});

if (video) {
  await cdp.send('Page.stopScreencast');
  await page.waitForTimeout(300);
  // Frames come only when the page changes: each lasts until the next one.
  // Frames closer than 1/25 s to the one kept before are dropped (the video
  // has 25 fps), so the durations add up to the recording's own clock.
  // A dropped frame's picture replaces the kept one's (less than 40 ms early).
  const sparse = [];
  for (const f of frames) {
    if (!sparse.length || f.t - sparse.at(-1).t >= 0.04) sparse.push({ ...f });
    else sparse.at(-1).data = f.data;
  }
  const list = [];
  sparse.forEach((f, i) => {
    const name = `frame-${String(i).padStart(5, '0')}.png`;
    writeFileSync(join(out, name), Buffer.from(f.data, 'base64'));
    const next = sparse[i + 1] ? sparse[i + 1].t : f.t + 1;
    list.push(`file '${name}'`, `duration ${(next - f.t).toFixed(4)}`);
  });
  list.push(`file 'frame-${String(sparse.length - 1).padStart(5, '0')}.png'`);
  writeFileSync(join(out, 'frames.txt'), list.join('\n') + '\n');
  const start = frames[0].t;
  const length = frames.at(-1).t - start + 1;
  console.log('frames', frames.length, 'seconds', length.toFixed(1), 'clock offset', (t0 - start).toFixed(3));

  // WebVTT from the cues, on the frames' clock.
  const stamp = (s) => {
    const ms = Math.max(0, Math.round(s * 1000));
    const p = (n, w = 2) => String(n).padStart(w, '0');
    return `${p(Math.floor(ms / 3600000))}:${p(Math.floor(ms / 60000) % 60)}:${p(Math.floor(ms / 1000) % 60)}.${p(ms % 1000, 3)}`;
  };
  const text = (h) => h.replace(/<br>/g, '\n').replace(/<[^>]+>/g, '').replace(/&amp;/g, '&');
  const vtt = ['WEBVTT', ''];
  cues.forEach((c, i) => {
    vtt.push(String(i + 1), `${stamp(c.start - start)} --> ${stamp((c.end ?? frames.at(-1).t) - start)}`, text(c.html), '');
  });
  writeFileSync(join(out, 'demo.vtt'), vtt.join('\n'));
  writeFileSync(join(out, 'cues.json'), JSON.stringify(cues.map((c) => ({ start: +(c.start - start).toFixed(3), end: +((c.end ?? frames.at(-1).t) - start).toFixed(3), text: text(c.html) })), null, 1));
  console.log('cues', cues.length);

  const ffmpeg = (args) => {
    const r = spawnSync('ffmpeg', ['-y', '-loglevel', 'error', ...args], { cwd: out, stdio: 'inherit' });
    if (r.status !== 0) throw new Error(`ffmpeg ${args.join(' ')} failed`);
  };
  ffmpeg(['-f', 'concat', '-safe', '0', '-i', 'frames.txt', '-i', 'demo.vtt', '-map', '0:v', '-map', '1:s',
    '-vf', 'fps=25,format=yuv420p', '-c:v', 'libx264', '-preset', 'slow', '-crf', '28', '-tune', 'stillimage',
    '-c:s', 'mov_text', '-metadata:s:s:0', 'language=eng', '-metadata:s:s:0', 'title=English',
    '-movflags', '+faststart', 'demo.mp4']);
  // The GIF: a highlight cut of the sections marked above.
  const parts = sections.map((s) => ({ ss: s.start - start, t: s.seconds }));
  writeFileSync(join(out, 'gif-sections.json'), JSON.stringify(parts));
  const inputs = parts.flatMap((p) => ['-ss', p.ss.toFixed(2), '-t', String(p.t), '-i', 'demo.mp4']);
  const chain = parts.map((_, i) => `[${i}:v]`).join('') + `concat=n=${parts.length}:v=1:a=0,fps=8,scale=960:-1:flags=lanczos,split[a][b];` +
    '[a]palettegen=max_colors=96:stats_mode=diff[p];[b][p]paletteuse=dither=bayer:bayer_scale=4:diff_mode=rectangle';
  ffmpeg([...inputs, '-filter_complex', chain, 'demo.gif']);
  console.log('wrote demo.mp4, demo.vtt, demo.gif');
}
await context.close();
await browser.close();
console.log(failures ? `done with ${failures} failed steps` : 'done');
process.exit(failures ? 1 : 0);
