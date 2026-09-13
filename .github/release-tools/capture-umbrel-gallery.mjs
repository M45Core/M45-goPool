import { chromium } from "playwright";
import path from "node:path";

const targetURL = process.env.GOPOOL_SCREENSHOT_URL;
const outputPath = process.env.GOPOOL_SCREENSHOT_PATH;
const chromiumPath = process.env.GOPOOL_CHROMIUM_PATH;

if (!targetURL || !outputPath) {
  throw new Error("GOPOOL_SCREENSHOT_URL and GOPOOL_SCREENSHOT_PATH are required");
}

const now = Date.now();
const isoMinutesAgo = (minutes) => new Date(now - minutes * 60_000).toISOString();

// These deterministic demo values are intentionally synthetic. Their scale is
// based on the public M45Core pool so release screenshots look representative
// without publishing a point-in-time copy of real worker data.
const overview = {
  api_version: "1",
  active_miners: 27,
  active_tls_miners: 0,
  shares_per_minute: 227,
  pool_hashrate: 27_900_000_000_000,
  pool_tag: "/m45usa-goPool/",
  btc_price_fiat: 77_850,
  btc_price_updated_at: isoMinutesAgo(1),
  fiat_currency: "usd",
  render_duration: 21_400,
  workers: [
    {
      name: "bc1qdemo...miner01.Axe01",
      display_name: "bc1qdemo...miner01.Axe01",
      rolling_hashrate: 1_540_000_000_000,
      difficulty: 1976,
      vardiff: 1976,
      share_rate: 10.9,
      accepted: 6429,
      connection_id: "A",
    },
    {
      name: "bc1qdemo...miner02.Axe02",
      display_name: "bc1qdemo...miner02.Axe02",
      rolling_hashrate: 1_150_000_000_000,
      hashrate_accuracy: "≈",
      difficulty: 1283,
      vardiff: 1283,
      share_rate: 10.5,
      accepted: 6207,
      connection_id: "B",
    },
    {
      name: "bc1qdemo...miner03.Axe03",
      display_name: "bc1qdemo...miner03.Axe03",
      rolling_hashrate: 926_000_000_000,
      difficulty: 899,
      vardiff: 899,
      share_rate: 9.7,
      accepted: 5716,
      connection_id: "C",
    },
  ],
  banned_workers: [],
  best_shares: [
    {
      worker: "bc1qdemo...miner01.Axe01",
      difficulty: 415_300_000_000,
      timestamp: isoMinutesAgo(88),
      hash: "...e842cf3987d77b6f",
    },
    {
      worker: "bc1qdemo...miner02.Axe02",
      difficulty: 103_700_000_000,
      timestamp: isoMinutesAgo(390),
      hash: "...eb51b869c6332c78",
    },
  ],
  miner_types: [
    {
      name: "bitaxe",
      total_workers: 21,
      versions: [
        { version: "BM1370/v2.15.1", workers: 12 },
        { version: "BM1366/v2.14.2", workers: 9 },
      ],
    },
    { name: "cgminer", total_workers: 6, versions: [{ version: "4.11.1", workers: 6 }] },
  ],
};

const hashrateHistory = Array.from({ length: 96 }, (_, index) => {
  const wave = Math.sin(index / 7) + Math.sin(index / 17) * 0.5;
  return wave > 0.65 ? 144 : wave < -0.65 ? 142 : 143;
});

const poolHashrate = {
  api_version: "1",
  pool_hashrate: overview.pool_hashrate,
  phh: hashrateHistory,
  block_height: 947_520,
  block_difficulty: 135_600_000_000_000,
  block_time_left_sec: 446,
  recent_block_times: [isoMinutesAgo(37), isoMinutesAgo(29), isoMinutesAgo(18), isoMinutesAgo(4)],
  next_difficulty_retarget: {
    height: 949_536,
    blocks_away: 2016,
    duration_estimate: "2 weeks",
  },
  template_tx_fees_sats: 682_595,
  template_updated_at: isoMinutesAgo(1),
  updated_at: isoMinutesAgo(1),
};

const browser = await chromium.launch({
  headless: true,
  ...(chromiumPath ? { executablePath: chromiumPath } : {}),
});
try {
  const page = await browser.newPage({
    viewport: { width: 1440, height: 900 },
    deviceScaleFactor: 1,
    colorScheme: "dark",
    locale: "en-US",
    timezoneId: "UTC",
    reducedMotion: "reduce",
  });

  await page.route("**/api/overview", (route) => route.fulfill({ json: overview }));
  await page.route("**/api/pool-hashrate*", (route) => route.fulfill({ json: poolHashrate }));
  await page.route("**/api/blocks*", (route) => route.fulfill({ json: [] }));

  await page.goto(targetURL, { waitUntil: "networkidle" });
  await page.waitForFunction(() => {
    const height = document.querySelector("#status-block-height")?.textContent?.trim();
    const hashrate = document.querySelector("#status-pool-hashrate")?.textContent?.trim();
    return height && height !== "--" && hashrate && hashrate !== "---";
  });

  await page.screenshot({
    path: path.resolve(outputPath),
    type: "png",
    fullPage: false,
  });
} finally {
  await browser.close();
}
