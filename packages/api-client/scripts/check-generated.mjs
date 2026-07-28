import { spawnSync } from "node:child_process";
import { readFileSync, rmSync, mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const scriptDirectory = dirname(fileURLToPath(import.meta.url));
const packageDirectory = resolve(scriptDirectory, "..");
const specification = resolve(packageDirectory, "../../api/openapi.yaml");
const committed = resolve(packageDirectory, "src/generated/schema.ts");
const temporaryDirectory = mkdtempSync(join(tmpdir(), "sewa-motor-openapi-"));
const candidate = join(temporaryDirectory, "schema.ts");

try {
  const result = spawnSync(
    "pnpm",
    ["exec", "openapi-typescript", specification, "--output", candidate],
    {
      cwd: packageDirectory,
      encoding: "utf8",
    },
  );

  if (result.status !== 0) {
    if (result.stdout) process.stderr.write(result.stdout);
    if (result.stderr) process.stderr.write(result.stderr);
    process.exit(result.status ?? 1);
  }

  if (readFileSync(committed, "utf8") !== readFileSync(candidate, "utf8")) {
    console.error(
      "Generated API types are stale. Run `pnpm generate` and commit the result.",
    );
    process.exit(1);
  }

  console.log("Generated API types match api/openapi.yaml.");
} finally {
  rmSync(temporaryDirectory, { recursive: true, force: true });
}
