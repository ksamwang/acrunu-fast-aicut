import { expect, test } from "@playwright/test";

const user = {
  id: "user-1",
  username: "test",
  display_name: "测试用户",
  role: "user"
};

function createAsset(id: string) {
  return {
    id,
    product_id: "product-1",
    asset_name: `素材 ${id}`,
    storage_key: `assets/${id}.mp4`,
    file_name: `${id}.mp4`,
    source_type: "visual_only",
    status: "ready",
    analysis_status: "ready",
    usability_status: "usable",
    duration_ms: 3200,
    width: 1080,
    height: 1920,
    has_audio: false,
    scene_description: "束裤带固定裤脚",
    shot_size: "close_up",
    camera_movement: "static",
    subjects: ["束裤带"],
    scene_tags: ["裤脚固定"],
    quality_tags: []
  };
}

test("queues every selected asset for VLM reanalysis", async ({ page }) => {
  let assets = [createAsset("asset-1"), createAsset("asset-2")];

  await page.addInitScript((session) => {
    window.localStorage.setItem("aicut.session", JSON.stringify(session));
  }, { token: "test-token", user });

  await page.route("**/storage/**", async (route) => {
    await route.fulfill({ status: 200, contentType: "video/mp4", body: "" });
  });
  await page.route((url) => url.pathname.startsWith("/api/"), async (route) => {
    const requestURL = new URL(route.request().url());
    if (requestURL.pathname === "/api/auth/me") {
      await route.fulfill({ contentType: "application/json", body: JSON.stringify({ data: user }) });
      return;
    }
    if (requestURL.pathname === "/api/products") {
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({ data: [{ id: "product-1", name: "束裤带", status: "active" }] })
      });
      return;
    }
    if (requestURL.pathname === "/api/assets/selection") {
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({ data: { asset_ids: assets.map((asset) => asset.id), total: assets.length } })
      });
      return;
    }
    if (requestURL.pathname === "/api/assets/bulk-reanalyze" && route.request().method() === "POST") {
      const body = route.request().postDataJSON() as { asset_ids: string[] };
      const selected = new Set(body.asset_ids);
      assets = assets.map((asset) => selected.has(asset.id) ? { ...asset, analysis_status: "pending_analysis" } : asset);
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            queued: assets.filter((asset) => selected.has(asset.id)).map((asset) => ({ asset, frame_task_id: `task-${asset.id}` })),
            skipped_ids: [],
            failures: []
          }
        })
      });
      return;
    }
    if (requestURL.pathname === "/api/assets") {
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({ data: { items: assets, total: assets.length, page: 1, page_size: 20 } })
      });
      return;
    }
    await route.fulfill({ contentType: "application/json", body: JSON.stringify({ data: [] }) });
  });

  await page.goto("/#/assets");
  await page.getByRole("button", { name: "批量选择" }).click();
  await page.getByRole("button", { name: "全选当前结果" }).click();
  await expect(page.getByText("已选 2 项", { exact: true })).toBeVisible();

  const requestPromise = page.waitForRequest((request) => request.url().endsWith("/api/assets/bulk-reanalyze") && request.method() === "POST");
  await page.getByRole("button", { name: "批量 VLM" }).click();
  const confirm = page.locator(".ant-modal-confirm").filter({ hasText: "重新分析 2 个素材？" });
  await expect(confirm).toBeVisible();
  await confirm.getByRole("button", { name: "开始 VLM" }).click();

  const request = await requestPromise;
  expect(new Set((request.postDataJSON() as { asset_ids: string[] }).asset_ids)).toEqual(new Set(["asset-1", "asset-2"]));
  await expect(page.getByRole("button", { name: "批量选择" })).toBeVisible();
  await expect(page.getByTestId("asset-card-asset-1").getByText("待分析", { exact: true })).toBeVisible();
  await expect(page.getByTestId("asset-card-asset-2").getByText("待分析", { exact: true })).toBeVisible();
});
