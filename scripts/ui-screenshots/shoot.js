// Screenshots of llama-swap's web UI for the Features tour's web UI slide.
// Run it after traffic.sh, with llama-swap serving config.yaml on :8080.
// See README.md. Writes 2x PNGs to out/.
const fs = require('fs');
const { chromium } = require('playwright');
fs.mkdirSync('out', { recursive: true });
const U = 'http://localhost:8080/ui/#';
(async () => {
  const b = await chromium.launch();
  const ctx = await b.newContext({ viewport: { width: 1440, height: 860 }, deviceScaleFactor: 2, colorScheme: 'light' });
  const p = await ctx.newPage();

  // Playground: a real chat against a fake model.
  await p.goto(U + '/playground'); await p.waitForTimeout(1500);
  const pick = p.getByPlaceholder('Select a model...');
  await pick.click(); await pick.fill('qwen3-8b'); await p.waitForTimeout(400); await p.keyboard.press('Enter');
  await p.waitForTimeout(400);
  await p.getByPlaceholder('Type a message...').fill('What does llama-swap do when I ask for a model that is not loaded?');
  await p.keyboard.press('Enter');
  await p.waitForTimeout(9000); // load + stream
  await p.screenshot({ path: 'out/playground.png' });

  // Activity
  await p.goto(U + '/'); await p.waitForTimeout(2500);
  await p.getByText('In-flight requests', { exact: false }).locator('xpath=ancestor::div[.//button][1]').locator('button').last().click().catch(e => console.log('no close', e.message));
  await p.waitForTimeout(800);
  await p.screenshot({ path: 'out/activity.png' });

  // Request inspector: open the newest request.
  await p.getByRole('button', { name: 'View', exact: true }).first().click(); await p.waitForTimeout(1500);
  await p.screenshot({ path: 'out/inspect.png' });
  await p.keyboard.press('Escape');

  // Models
  await p.goto(U + '/models'); await p.waitForTimeout(2000);
  await p.screenshot({ path: 'out/models.png' });

  // Logs
  await p.goto(U + '/logs'); await p.waitForTimeout(2500);
  await p.screenshot({ path: 'out/logs.png' });
  await b.close();
})();
