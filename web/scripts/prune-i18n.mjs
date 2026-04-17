// One-shot: remove orphan keys from translations.ts across all locales.
// Handles both single-line ("key": "value",) and multi-line
// ("key":\n    "value",) entries.
//
// Usage:  node scripts/prune-i18n.mjs
import fs from "node:fs";
import path from "node:path";

const FILE = path.resolve("src/i18n/translations.ts");
const UNUSED = new Set([
  "about.features",
  "about.links",
  "cli.divider",
  "code.copied",
  "feature.archiveDownload",
  "feature.cors",
  "feature.encryptionDesc",
  "feature.expirationDesc",
  "feature.keybase",
  "feature.profiler",
  "feature.proxy",
  "feature.qrCode",
  "feature.s3Compat",
  "feature.selfHostedDesc",
  "feature.unlimitedUpload",
  "qa.a1",
  "qa.a2",
  "qa.a3",
  "qa.a4",
  "qa.a5",
  "qa.a6",
  "qa.divider",
  "qa.q1",
  "qa.q2",
  "qa.q3",
  "qa.q4",
  "qa.q5",
  "qa.q6",
  "upload.browse",
  "upload.failed",
  "usecase.dbBackupShort",
  "usecase.divider",
  "usecase.gpgEncryptShort",
  "usecase.malwareScanShort",
  "usecase.viewAll",
]);

const src = fs.readFileSync(FILE, "utf8");
const lines = src.split("\n");
const out = [];
let removed = 0;

for (let i = 0; i < lines.length; i++) {
  const line = lines[i];
  const m = line.match(/^\s*"([a-zA-Z0-9._]+)"\s*:(.*)$/);

  if (m && UNUSED.has(m[1])) {
    removed++;
    const rest = m[2];
    // Inline entry? Value + comma on the same line.
    if (/",\s*(\/\/.*)?$/.test(rest)) {
      continue; // skip this one line
    }
    // Multi-line entry — consume until we see a line ending with `",`.
    while (i < lines.length && !/",\s*(\/\/.*)?$/.test(lines[i])) i++;
    continue; // for-loop i++ will skip the closing line too
  }

  out.push(line);
}

fs.writeFileSync(FILE, out.join("\n"));
console.log(`Removed ${removed} key entries. Lines: ${lines.length} -> ${out.length}`);
