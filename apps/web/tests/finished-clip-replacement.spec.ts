import { expect, test, type Page, type Route } from "@playwright/test";

const user = {
  id: "user-1",
  username: "test",
  display_name: "测试用户",
  role: "user"
};

const work = {
  id: "work-1",
  run_id: "work-1",
  generation_batch_id: "batch-1",
  product_id: "product-1",
  product_name: "束裤带",
  created_by_name: "测试用户",
  title: "替换镜头测试",
  hook: "替换镜头测试",
  script_text: "展示产品动作和使用效果。",
  duration_ms: 6_000,
  status: "completed",
  progress: 100,
  stage_label: "已完成",
  created_at: "2026-08-01T10:00:00.000Z",
  completed_at: "2026-08-01T10:01:00.000Z",
  video_url: "/storage/work-1.mp4",
  edit_plan_updated_at: "2026-08-01T10:01:00.000Z",
  edit_plan: [
    {
      id: "clip-1",
      visual_beat_id: "beat-1",
      narration_segment_id: "segment-1",
      asset_id: "asset-current",
      start_ms: 0,
      end_ms: 2_000,
      source_in_ms: 0,
      source_out_ms: 2_000,
      label: "展示弹力",
      visual_goal: "双手拉伸束裤带展示弹力",
      source_type: "visual_only"
    },
    {
      id: "clip-2",
      visual_beat_id: "beat-2",
      narration_segment_id: "segment-2",
      asset_id: "asset-used",
      start_ms: 2_000,
      end_ms: 4_000,
      source_in_ms: 0,
      source_out_ms: 2_000,
      label: "穿戴效果",
      visual_goal: "束裤带固定裤脚",
      source_type: "visual_only"
    },
    {
      id: "clip-3",
      visual_beat_id: "beat-3",
      narration_segment_id: "segment-3",
      asset_id: "asset-final",
      start_ms: 4_000,
      end_ms: 6_000,
      source_in_ms: 0,
      source_out_ms: 2_000,
      label: "骑行效果",
      visual_goal: "裤脚远离自行车链条",
      source_type: "visual_only"
    }
  ]
};

const candidates = {
  clip_id: "clip-1",
  query: "双手拉伸束裤带展示弹力",
  clip_duration_ms: 2_000,
  plan_updated_at: "2026-08-01T10:01:00.000Z",
  current: {
    asset_id: "asset-current",
    asset_name: "当前弹力素材.mp4",
    file_name: "current.mp4",
    source_type: "visual_only",
    duration_ms: 2_500,
    source_in_ms: 0,
    max_source_in_ms: 500,
    video_url: "/storage/current.mp4",
    semantic_score: 0.88,
    is_current: true,
    selectable: true
  },
  items: [
    {
      asset_id: "asset-used",
      asset_name: "高分已占用素材.mp4",
      file_name: "used.mp4",
      source_type: "visual_only",
      duration_ms: 3_000,
      source_in_ms: 0,
      max_source_in_ms: 1_000,
      video_url: "/storage/used.mp4",
      action_description: "双手持续拉伸束裤带",
      semantic_score: 0.95,
      is_current: false,
      selectable: false,
      unavailable_reason: "已被镜头 02 使用"
    },
    {
      asset_id: "asset-short",
      asset_name: "短素材.mp4",
      file_name: "short.mp4",
      source_type: "visual_only",
      duration_ms: 1_820,
      source_in_ms: 0,
      max_source_in_ms: 0,
      video_url: "/storage/short.mp4",
      action_description: "双手拉伸束裤带展示回弹",
      semantic_score: 0.91,
      is_current: false,
      selectable: true,
      shortfall_ms: 180
    },
    {
      asset_id: "asset-full",
      asset_name: "完整素材.mp4",
      file_name: "full.mp4",
      source_type: "visual_only",
      duration_ms: 2_500,
      source_in_ms: 0,
      max_source_in_ms: 500,
      video_url: "/storage/full.mp4",
      action_description: "手持束裤带进行弹力演示",
      semantic_score: 0.89,
      is_current: false,
      selectable: true
    }
  ]
};

async function fulfillJSON(route: Route, data: unknown) {
  await route.fulfill({
    contentType: "application/json",
    body: JSON.stringify({ data })
  });
}

async function installMocks(page: Page) {
  await page.addInitScript((session) => {
    window.localStorage.setItem("aicut.session", JSON.stringify(session));
  }, { token: "finished-clip-token", user });

  await page.route("**/storage/**", async (route) => {
    await route.fulfill({ status: 204 });
  });

  await page.route((url) => url.pathname.startsWith("/api/"), async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname === "/api/auth/me") {
      await fulfillJSON(route, user);
      return;
    }
    if (url.pathname === "/api/products") {
      await fulfillJSON(route, [{ id: "product-1", name: "束裤带", status: "active" }]);
      return;
    }
    if (url.pathname === "/api/workbench/works") {
      await fulfillJSON(route, [work]);
      return;
    }
    if (url.pathname === "/api/workbench/works/work-1/clips/clip-1/candidates") {
      await fulfillJSON(route, candidates);
      return;
    }
    await fulfillJSON(route, []);
  });
}

test("keeps semantic order and allows a short replacement with an early transition", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 });
  await installMocks(page);
  await page.goto("/#/finished/work-1");

  const detail = page.getByTestId("finished-work-detail");
  await expect(detail).toBeVisible();
  await detail.getByRole("tab", { name: /镜头编排/ }).click();
  await detail.getByRole("button", { name: "替换", exact: true }).first().click();

  const panel = detail.getByRole("region", { name: "替换镜头素材" });
  await expect(panel.getByRole("button", { name: /当前素材 当前弹力素材\.mp4/ })).toBeVisible();

  const rows = panel.locator(".finished-detail-candidate-row");
  await expect(rows).toHaveCount(3);
  await expect(rows.nth(0)).toContainText("高分已占用素材.mp4");
  await expect(rows.nth(1)).toContainText("短素材.mp4");
  await expect(rows.nth(2)).toContainText("完整素材.mp4");

  await rows.nth(0).click();
  await expect(panel.locator(".finished-detail-candidate-footer").getByText("已被镜头 02 使用", { exact: true })).toBeVisible();
  await expect(panel.getByRole("button", { name: "不可选" })).toBeDisabled();

  await rows.nth(1).click();
  await expect(panel.getByText("取用 0:00.000 - 0:01.820 · 提前转场 180ms", { exact: true })).toBeVisible();
  await expect(panel.getByRole("button", { name: "使用此素材" })).toBeEnabled();
  await panel.getByRole("button", { name: "使用此素材" }).click();

  await expect(detail.getByText("替换为 短素材.mp4 · 提前转场 180ms", { exact: true })).toBeVisible();
  await expect(detail.getByText("已修改 1 个镜头", { exact: true })).toBeVisible();
});
