// Playwright config. Resets a scratch database, loads the demo data
// (scripts/demo-data.sql), builds the packrat binary and starts it on a
// random port.
//
// Needs PACKRAT_E2E_DATABASE_URL: a PostgreSQL database whose name ends in
// _e2e, which each run wipes (cmd/devtool refuses any other name), e.g.
//
//   createdb packrat_e2e
//   PACKRAT_E2E_DATABASE_URL='postgres:///packrat_e2e?host=/var/run/postgresql' npx playwright test
import { defineConfig } from '@playwright/test';
import { mkdtempSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

const DSN = process.env.PACKRAT_E2E_DATABASE_URL;
if (!DSN) throw new Error('Set PACKRAT_E2E_DATABASE_URL to a scratch database whose name ends in _e2e (it is wiped on every run).');

// This file is evaluated again in each worker process, so the chosen values
// are pinned into the environment on first evaluation and reused afterwards.
if (!process.env.PACKRAT_E2E_PORT) process.env.PACKRAT_E2E_PORT = String(20000 + Math.floor(Math.random() * 10000));
if (!process.env.PACKRAT_E2E_DIR) process.env.PACKRAT_E2E_DIR = mkdtempSync(join(tmpdir(), 'packrat-e2e-'));
const PORT = process.env.PACKRAT_E2E_PORT;
const bin = join(process.env.PACKRAT_E2E_DIR, 'packrat');
process.env.PACKRAT_E2E_BASE = `http://127.0.0.1:${PORT}`;

export default defineConfig({
  testDir: './tests',
  timeout: 30_000,
  fullyParallel: false,
  workers: 1,
  reporter: [['list']],
  use: {
    baseURL: process.env.PACKRAT_E2E_BASE,
    viewport: { width: 1500, height: 900 },
    screenshot: 'only-on-failure',
  },
  webServer: {
    command: `cd .. && go run ./cmd/devtool reset "${DSN}" && go run ./cmd/devtool demo "${DSN}" && go build -o ${bin} . && ${bin} -addr 127.0.0.1:${PORT} -db "${DSN}"`,
    url: process.env.PACKRAT_E2E_BASE + '/api/version',
    reuseExistingServer: false,
    timeout: 180_000,
  },
  projects: [{ name: 'chromium', use: { browserName: 'chromium' } }],
});
