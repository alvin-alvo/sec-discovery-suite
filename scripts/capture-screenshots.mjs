import puppeteer from 'puppeteer-core'
import fs from 'fs'
import path from 'path'

const CHROME_PATH = '/home/alvo/projects/sec-discovery-suite/.chrome/chrome/linux-155.0.8059.39/chrome-linux64/chrome'
const OUTPUT_DIR = '/home/alvo/projects/sec-discovery-suite/docs/images'

const FS_RUN_ID = '01M4NMY68PD1KYDMVCGWW3B7KQ'
const TLS_RUN_ID = '01M4NM4M34AWNG7JA63Q6SDG3J'
const BASE_URL = 'http://127.0.0.1:8080'

fs.mkdirSync(OUTPUT_DIR, { recursive: true })

async function capture() {
  console.log('Launching Chrome...')
  const browser = await puppeteer.launch({
    executablePath: CHROME_PATH,
    headless: true,
    args: ['--no-sandbox', '--disable-setuid-sandbox', '--disable-dev-shm-usage', '--window-size=1440,960'],
    defaultViewport: { width: 1440, height: 960, deviceScaleFactor: 2 },
  })

  const page = await browser.newPage()

  // 1. Dashboard Overview
  console.log('Capturing 01_dashboard_overview.png...')
  await page.goto(`${BASE_URL}/runs`, { waitUntil: 'networkidle0' })
  await page.waitForSelector('table', { timeout: 10000 })
  await new Promise((r) => setTimeout(r, 2000))
  await page.screenshot({ path: path.join(OUTPUT_DIR, '01_dashboard_overview.png') })

  // 2. System Information & Documentation Modal (clicking "i" button)
  console.log('Capturing 02_system_info_modal.png...')
  const infoBtn = await page.waitForSelector('button[aria-label="System Information"]')
  if (infoBtn) {
    await infoBtn.click()
    await page.waitForSelector('#system-info-title', { timeout: 5000 })
    await new Promise((r) => setTimeout(r, 1000))
    await page.screenshot({ path: path.join(OUTPUT_DIR, '02_system_info_modal.png') })
    // Close modal
    const closeBtn = await page.waitForSelector('button[aria-label="Close dialog"]')
    if (closeBtn) await closeBtn.click()
    await new Promise((r) => setTimeout(r, 500))
  }

  // 3. Filesystem Discovery Report
  console.log('Capturing 03_fs_discovery_report.png...')
  await page.goto(`${BASE_URL}/runs/${FS_RUN_ID}/fs`, { waitUntil: 'networkidle0' })
  await page.waitForSelector('table', { timeout: 10000 })
  await new Promise((r) => setTimeout(r, 2000))
  await page.screenshot({ path: path.join(OUTPUT_DIR, '03_fs_discovery_report.png') })

  // 4. Certificate Inspection Drawer (click on a valid certificate row)
  console.log('Capturing 04_cert_inspection_drawer.png...')
  const validRow = await page.waitForSelector('table tbody tr:nth-child(4)')
  if (validRow) {
    await validRow.click()
    await new Promise((r) => setTimeout(r, 1500))
    await page.screenshot({ path: path.join(OUTPUT_DIR, '04_cert_inspection_drawer.png') })
  }

  // 5. TLS Metadata Scan Report
  console.log('Capturing 05_tls_metadata_report.png...')
  await page.goto(`${BASE_URL}/runs/${TLS_RUN_ID}/tls`, { waitUntil: 'networkidle0' })
  await page.waitForSelector('table', { timeout: 10000 })
  await new Promise((r) => setTimeout(r, 2500))
  await page.screenshot({ path: path.join(OUTPUT_DIR, '05_tls_metadata_report.png') })

  // 6. Cryptographic Relationship Map
  console.log('Capturing 06_cryptographic_relationship_map.png...')
  await page.evaluate(() => {
    const el = document.querySelector('.react-flow')
    if (el) el.scrollIntoView({ behavior: 'instant', block: 'center' })
  })
  await new Promise((r) => setTimeout(r, 2000))
  await page.screenshot({ path: path.join(OUTPUT_DIR, '06_cryptographic_relationship_map.png') })

  // 7. Expanded Endpoint Flows Inspection
  console.log('Capturing 07_endpoint_flow_details.png...')
  const firstEndpoint = await page.waitForSelector('table tbody tr:first-child')
  if (firstEndpoint) {
    await firstEndpoint.click()
    await page.evaluate(() => {
      const el = document.querySelector('table tbody tr:first-child')
      if (el) el.scrollIntoView({ behavior: 'instant', block: 'center' })
    })
    await new Promise((r) => setTimeout(r, 1500))
    await page.screenshot({ path: path.join(OUTPUT_DIR, '07_endpoint_flow_details.png') })
  }


  await browser.close()
  console.log('All screenshots captured successfully!')
}

capture().catch((err) => {
  console.error('Error capturing screenshots:', err)
  process.exit(1)
})
