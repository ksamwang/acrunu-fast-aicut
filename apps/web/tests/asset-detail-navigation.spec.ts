import { expect, test, type Page, type Route } from "@playwright/test";

const assets = Array.from({ length: 21 }, (_, index) => ({
  id: `asset-${index + 1}`,
  product_id: "product-1",
  asset_name: `素材 ${index + 1}`,
  storage_key: `assets/asset-${index + 1}.mp4`,
  file_name: `asset-${index + 1}.mp4`,
  source_type: "visual_only",
  status: "ready",
  analysis_status: "ready",
  usability_status: "usable",
  duration_ms: 3000,
  width: 1080,
  height: 1920,
  has_audio: false,
  scene_description: `画面 ${index + 1}`,
  action_description: "人物手持束裤带，展示键盘图案设计细节",
  shot_size: "close_up",
  camera_movement: "static",
  subjects: ["产品"],
  scene_tags: [],
  quality_tags: [],
  reviewer_notes: "",
  updated_at: "2026-07-23T00:00:00Z"
}));

async function fulfillJSON(route: Route, data: unknown) {
  await route.fulfill({
    contentType: "application/json",
    body: JSON.stringify({ data })
  });
}

async function installMocks(page: Page) {
  await page.addInitScript(() => {
    window.localStorage.setItem("aicut.session", JSON.stringify({
      token: "asset-navigation-token",
      user: { id: "user-1", username: "test", display_name: "测试用户", role: "user" }
    }));
  });

  await page.route("**/storage/**", async (route) => {
    await route.fulfill({ status: 204 });
  });

  await page.route((url) => url.pathname.startsWith("/api/"), async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname === "/api/auth/me") {
      await fulfillJSON(route, { id: "user-1", username: "test", display_name: "测试用户", role: "user" });
      return;
    }
    if (url.pathname === "/api/products") {
      await fulfillJSON(route, [{ id: "product-1", name: "束裤带", status: "active" }]);
      return;
    }
    if (url.pathname === "/api/products/product-1/selling-points") {
      await fulfillJSON(route, []);
      return;
    }
    if (url.pathname === "/api/assets") {
      const pageNumber = Number(url.searchParams.get("page") || 1);
      const pageSize = Number(url.searchParams.get("page_size") || 20);
      const start = (pageNumber - 1) * pageSize;
      await fulfillJSON(route, {
        items: assets.slice(start, start + pageSize),
        total: assets.length,
        page: pageNumber,
        page_size: pageSize
      });
      return;
    }

    const reviewMatch = url.pathname.match(/^\/api\/assets\/(asset-\d+)\/review$/);
    if (reviewMatch && route.request().method() === "PUT") {
      const body = route.request().postDataJSON() as Record<string, unknown>;
      const asset = assets.find((item) => item.id === reviewMatch[1]);
      if (!asset) {
        await route.fulfill({ status: 404 });
        return;
      }
      Object.assign(asset, {
        scene_description: String(body.scene_description ?? ""),
        action_description: String(body.action_description ?? ""),
        shot_size: String(body.shot_size ?? ""),
        camera_movement: String(body.camera_movement ?? ""),
        subjects: (body.subjects as string[]) ?? [],
        scene_tags: (body.scene_tags as string[]) ?? [],
        quality_tags: (body.quality_tags as string[]) ?? [],
        usability_status: String(body.usability_status ?? ""),
        reviewer_notes: String(body.reviewer_notes ?? "")
      });
      await fulfillJSON(route, asset);
      return;
    }

    const detailMatch = url.pathname.match(/^\/api\/assets\/(asset-\d+)\/(frames|semantic-preview|embeddings|selling-points)$/);
    if (detailMatch) {
      const [, assetID, resource] = detailMatch;
      if (resource === "frames") {
        await fulfillJSON(route, { asset_id: assetID, frames: [] });
      } else if (resource === "semantic-preview") {
        const asset = assets.find((item) => item.id === assetID);
        const action = asset?.action_description || "";
        await fulfillJSON(route, {
          asset_id: assetID,
          open_semantic_description: action ? `动作：${action}` : "",
          embedding_targets: action ? [{ object_type: "shot", object_id: assetID, asset_id: assetID, text: `动作：${action}` }] : []
        });
      } else if (resource === "embeddings") {
        await fulfillJSON(route, { items: [] });
      } else {
        await fulfillJSON(route, []);
      }
      return;
    }
    await fulfillJSON(route, []);
  });
}

test("navigates asset details across pages and protects unsaved review changes", async ({ page }) => {
  await installMocks(page);
  await page.goto("/#/assets");

  await expect(page.getByTestId("assets-page")).toBeVisible();
  await page.getByRole("button", { name: "素材 20", exact: true }).click();
  const detail = page.getByTestId("asset-detail-modal");
  await expect(detail).toBeVisible();
  await expect(detail.locator(".asset-detail-navigation-count")).toHaveText("20 / 21");

  await detail.getByRole("button", { name: "编辑标签" }).click();
  await detail.getByLabel("画面描述").fill("尚未保存的修改");
  await detail.getByRole("button", { name: "下一条素材" }).click();
  await expect(page.locator(".ant-modal-confirm-title").filter({ hasText: "放弃未保存的修改？" })).toBeVisible();
  await page.getByRole("button", { name: "放弃并切换" }).click();

  await expect(detail.getByRole("heading", { name: "素材 21" })).toBeVisible();
  await expect(detail.locator(".asset-detail-navigation-count")).toHaveText("21 / 21");
  await expect(page.locator(".asset-pagination .ant-pagination-item-active")).toHaveText("2");

  await detail.getByRole("button", { name: "上一条素材" }).click();
  await expect(detail.getByRole("heading", { name: "素材 20" })).toBeVisible();
  await expect(detail.locator(".asset-detail-navigation-count")).toHaveText("20 / 21");
  await expect(page.locator(".asset-pagination .ant-pagination-item-active")).toHaveText("1");

  await detail.getByRole("button", { name: "编辑标签" }).click();
  await detail.getByLabel("动作描述").fill("人物双手反复拉伸和放松束裤带，展示弹性");
  await detail.getByRole("button", { name: "保存复核" }).click();
  await expect(detail.getByTestId("asset-analysis-panel")).toContainText("人物双手反复拉伸和放松束裤带，展示弹性");
  await detail.getByRole("tab", { name: "向量预览" }).click();
  await expect(detail).toContainText("动作：人物双手反复拉伸和放松束裤带，展示弹性");
});
