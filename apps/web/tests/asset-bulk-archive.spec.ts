import { expect, test } from "@playwright/test";

const user = {
  id: "user-1",
  username: "test",
  display_name: "测试用户",
  role: "user"
};

function createAsset(id: string, name: string, status = "ready") {
  return {
    id,
    product_id: "product-1",
    asset_name: name,
    storage_key: `assets/${id}.mp4`,
    file_name: `${id}.mp4`,
    source_type: "visual_only",
    status,
    analysis_status: "ready",
    usability_status: "usable",
    duration_ms: 3200,
    width: 1080,
    height: 1920,
    has_audio: false,
    scene_description: `${name}的画面描述`,
    shot_size: "close_up",
    camera_movement: "static",
    subjects: ["产品"],
    scene_tags: ["展示"],
    quality_tags: [],
    updated_at: "2026-07-30T00:00:00.000Z",
    analyzed_at: "2026-07-30T00:00:00.000Z",
    ...(status === "archived" ? { archived_at: "2026-07-30T00:01:00.000Z" } : {})
  };
}

test("selects one or all active assets and archives them", async ({ page }) => {
  let assets = [
    createAsset("asset-1", "素材 A"),
    createAsset("asset-2", "素材 B"),
    createAsset("asset-archived", "历史素材", "archived")
  ];

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
      const assetIDs = assets.filter((asset) => asset.status !== "archived").map((asset) => asset.id);
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({ data: { asset_ids: assetIDs, total: assetIDs.length } })
      });
      return;
    }
    if (requestURL.pathname === "/api/assets/bulk-archive" && route.request().method() === "POST") {
      const body = route.request().postDataJSON() as { asset_ids: string[] };
      const selected = new Set(body.asset_ids);
      const archived = assets
        .filter((asset) => selected.has(asset.id) && asset.status !== "archived")
        .map((asset) => ({ ...asset, status: "archived", archived_at: "2026-07-30T00:02:00.000Z" }));
      const archivedByID = new Map(archived.map((asset) => [asset.id, asset]));
      assets = assets.map((asset) => archivedByID.get(asset.id) ?? asset);
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({ data: { archived, skipped_ids: [], failures: [] } })
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
  await expect(page.getByTestId("assets-page")).toBeVisible();
  await expect(page.getByTestId("asset-card-asset-archived")).toHaveCSS("background-color", "rgb(255, 245, 245)");

  await page.getByRole("button", { name: "批量选择" }).click();
  await expect(page.getByRole("checkbox", { name: "历史素材 已归档" })).toBeDisabled();
  await page.getByTestId("asset-card-asset-1").getByRole("button", { name: "选择 素材 A" }).click();
  await expect(page.getByText("已选 1 项", { exact: true })).toBeVisible();

  await page.getByRole("button", { name: "全选当前结果" }).click();
  await expect(page.getByText("已选 2 项", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "归档所选" }).click();
  const confirm = page.locator(".ant-modal-confirm").filter({ hasText: "归档 2 个素材？" });
  await expect(confirm).toBeVisible();
  await confirm.getByRole("button", { name: /归\s*档/ }).click();

  await expect(page.getByTestId("asset-card-asset-1")).toHaveAttribute("data-status", "archived");
  await expect(page.getByTestId("asset-card-asset-2")).toHaveAttribute("data-status", "archived");
  await expect(page.getByTestId("asset-card-asset-1")).toHaveCSS("background-color", "rgb(255, 245, 245)");
  await expect(page.getByRole("button", { name: "批量选择" })).toBeVisible();
});
