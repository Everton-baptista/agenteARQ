import { test, expect } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { spawn, spawnSync, type ChildProcess } from "node:child_process";
import { mkdtempSync, rmSync, existsSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";

const binary = resolve(process.env.AGENTARCH_BIN ?? "../bin/agentarch");
let root: string, child: ChildProcess, url: string;
function command(program: string, args: string[]) {
  const r = spawnSync(program, args, { encoding: "utf8" });
  if (r.status !== 0) throw new Error(r.stderr || "command failed");
}
test.beforeAll(async () => {
  root = mkdtempSync(join(tmpdir(), "agentarch-browser-"));
  command("git", ["init", "-q", root]);
  child = spawn(binary, ["ui", "--root", root, "--port", "0"], {
    stdio: ["ignore", "pipe", "pipe"],
  });
  url = await new Promise<string>((ok, fail) => {
    let output = "";
    const timer = setTimeout(
      () => fail(new Error("dashboard did not start")),
      10000,
    );
    child.once("error", (e) => {
      clearTimeout(timer);
      fail(e);
    });
    child.once("exit", () => {
      clearTimeout(timer);
      fail(new Error("dashboard exited"));
    });
    child.stdout!.on("data", (data) => {
      output += String(data);
      const match = output.match(
        /http:\/\/127\.0\.0\.1:\d+\/\?token=[a-f0-9]+/,
      );
      if (match) {
        clearTimeout(timer);
        ok(match[0]);
      }
    });
  });
});
test.afterAll(async () => {
  if (child?.exitCode === null) {
    const ended = new Promise<void>((r) => child.once("exit", () => r()));
    child.kill("SIGTERM");
    await ended;
  }
  if (root) rmSync(root, { recursive: true, force: true });
});

test("onboarding, live refresh, navigation, mobile and read-only API", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  const unauth = await page.request.get(new URL("/api/status", url).href);
  expect(unauth.status()).toBe(401);
  await page.goto(url);
  await expect(
    page.getByRole("heading", {
      name: "Seu próximo projeto começa com uma boa pergunta.",
    }),
  ).toBeVisible();
  expect(page.url()).not.toContain("token=");
  command(binary, [
    "workflow",
    "init",
    "--root",
    root,
    "--name",
    "Produto de teste",
    "--profile",
    "typescript-web",
    "--author",
    "Autora de teste",
    "--goal",
    "Consultar o progresso do produto com evidências verificáveis",
  ]);
  await page.getByRole("button", { name: "Atualizar", exact: true }).click();
  await expect(
    page.getByRole("heading", {
      name: "Defina o problema e os critérios de aceite.",
    }),
  ).toBeVisible();
  for (const [name, heading] of [
    ["Tarefas e critérios", "Tarefas e critérios de aceite"],
    ["Qualidade", "Última verificação"],
    ["Contexto", "Contrato do produto"],
  ]) {
    await page.getByRole("button", { name, exact: true }).click();
    await expect(
      page.getByRole("heading", { name: heading, exact: true }),
    ).toBeVisible();
  }
  const write = await page.request.post(new URL("/api/status", url).href, {
    data: { phase: "pr_ready" },
  });
  expect(write.status()).toBe(405);
  await page.setViewportSize({ width: 390, height: 844 });
  await page.getByRole("button", { name: "Visão geral", exact: true }).click();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  expect(errors).toEqual([]);
});

test("keyboard focus and WCAG AA accessibility", async ({ page }) => {
  if (!existsSync(join(root, "agentarch/project/product.yaml"))) {
    command(binary, [
      "workflow",
      "init",
      "--root",
      root,
      "--name",
      "Produto de teste",
      "--profile",
      "typescript-web",
      "--author",
      "Autora de teste",
      "--goal",
      "Consultar o progresso do produto com evidências verificáveis",
    ]);
  }

  await page.goto(url);
  await expect(
    page.getByRole("region", { name: "Resumo do projeto" }),
  ).toBeVisible();
  await page.keyboard.press("Tab");
  await expect(
    page.getByRole("link", { name: "Pular para o conteúdo" }),
  ).toBeFocused();
  const result = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
    .analyze();
  expect(
    result.violations.map((v) => ({
      id: v.id,
      nodes: v.nodes.map((n) => ({
        target: n.target,
        message: n.failureSummary,
      })),
    })),
  ).toEqual([]);
});
