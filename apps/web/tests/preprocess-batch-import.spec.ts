import { expect, test } from "@playwright/test";

type MockWorkspaceItem = {
  id: string;
  status: "pending";
  asset_name: string;
  source_type: "visual_only";
  original_file_name: string;
  source_file_name: string;
  source_file_size: number;
  source_in_ms: number;
  source_out_ms: number;
  transcript: string;
  transcript_segments: never[];
  reviewer_notes: string;
  probe: {
    duration_ms: number;
    width: number;
    height: number;
    fps: number;
    codec: string;
    has_audio: boolean;
    audio_codec: string;
  };
  frame_snapshots: never[];
  source_url: string;
  thumbnail_url: string;
  vlm_status: "idle";
  updated_at: string;
};

const transparentPixel = "data:image/gif;base64,R0lGODlhAQABAAD/ACwAAAAAAQABAAACADs=";

function workspaceItem(index: number, fileName: string): MockWorkspaceItem {
  const id = `workspace-${index}`;
  return {
    id,
    status: "pending",
    asset_name: "",
    source_type: "visual_only",
    original_file_name: fileName,
    source_file_name: fileName,
    source_file_size: 5,
    source_in_ms: 0,
    source_out_ms: 6000,
    transcript: "",
    transcript_segments: [],
    reviewer_notes: "",
    probe: {
      duration_ms: 6000,
      width: 1080,
      height: 1920,
      fps: 30,
      codec: "h264",
      has_audio: true,
      audio_codec: "aac"
    },
    frame_snapshots: [],
    source_url: `http://127.0.0.1:58721/workspace/items/${id}/source`,
    thumbnail_url: transparentPixel,
    vlm_status: "idle",
    updated_at: "2026-07-17T00:00:00.000Z"
  };
}

test("imports a large preprocess batch without blocking or mounting videos", async ({ page }) => {
  const importedItems: MockWorkspaceItem[] = [];
  let activeImports = 0;
  let maxActiveImports = 0;
  let importSequence = 0;

  await page.addInitScript(() => {
    window.localStorage.setItem("aicut.session", JSON.stringify({
      token: "batch-import-token",
      user: {
        id: "admin-1",
        username: "admin",
        display_name: "Admin",
        role: "admin"
      }
    }));
  });

  await page.route((url) => url.pathname.startsWith("/api/"), async (route) => {
    if (route.request().url().includes("/api/auth/me")) {
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            id: "admin-1",
            username: "admin",
            display_name: "Admin",
            role: "admin"
          }
        })
      });
      return;
    }
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ data: [] })
    });
  });

  await page.route("http://127.0.0.1:58721/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const corsHeaders = { "Access-Control-Allow-Origin": "*" };

    if (request.method() === "OPTIONS") {
      await route.fulfill({ status: 204, headers: corsHeaders });
      return;
    }
    if (request.method() === "GET" && url.pathname === "/workspace/items") {
      const pageNumber = Number(url.searchParams.get("page") ?? "1");
      const pageSize = Number(url.searchParams.get("page_size") ?? "50");
      const start = (pageNumber - 1) * pageSize;
      await route.fulfill({
        contentType: "application/json",
        headers: corsHeaders,
        body: JSON.stringify({
          items: importedItems.slice(start, start + pageSize),
          total: importedItems.length,
          page: pageNumber,
          page_size: pageSize,
          stats: {
            pending: importedItems.length,
            saved: 0,
            ready_to_submit: 0,
            submitted: 0
          },
          has_running_vlm: false
        })
      });
      return;
    }
    if (request.method() === "POST" && url.pathname === "/workspace/import") {
      activeImports += 1;
      maxActiveImports = Math.max(maxActiveImports, activeImports);
      const sequence = ++importSequence;
      try {
        await new Promise((resolve) => setTimeout(resolve, 120));
        const item = workspaceItem(sequence, `batch-${String(sequence).padStart(2, "0")}.mp4`);
        importedItems.push(item);
        await route.fulfill({
          contentType: "application/json",
          headers: corsHeaders,
          body: JSON.stringify({ items: [item] })
        });
      } finally {
        activeImports -= 1;
      }
      return;
    }
    await route.fulfill({ status: 404, contentType: "application/json", headers: corsHeaders, body: "{}" });
  });

  await page.goto("/#/preprocess");
  await expect(page.getByRole("menuitem", { name: "预处理" })).toHaveClass(/ant-menu-item-selected/);
  await page.locator(".preprocess-import-fab").click();

  const importModal = page.locator(".preprocess-import-modal");
  const files = Array.from({ length: 55 }, (_, index) => ({
    name: `batch-${String(index + 1).padStart(2, "0")}.mp4`,
    mimeType: "video/mp4",
    buffer: Buffer.from("video")
  }));
  await importModal.locator('input[type="file"]').setInputFiles(files);
  await expect(importModal.locator(".preprocess-import-preview-card")).toHaveCount(50);
  await expect(importModal.locator("video")).toHaveCount(0);

  await importModal.getByRole("button", { name: /开始导入/ }).click();
  await expect.poll(() => maxActiveImports).toBe(2);
  await importModal.locator(".preprocess-import-preview-card").nth(10).getByRole("button", { name: "移除" }).click();
  await importModal.locator(".preprocess-import-actions button").nth(1).click();
  await expect(importModal).not.toBeVisible();
  await expect(page.getByRole("button", { name: "刷新" })).toBeEnabled();

  await expect.poll(() => importedItems.length, { timeout: 15_000 }).toBe(54);
  expect(maxActiveImports).toBeLessThanOrEqual(2);
  await expect(page.locator(".preprocess-asset-card")).toHaveCount(4);
  await expect(page.locator(".preprocess-asset-grid video")).toHaveCount(0);

  await page.locator(".preprocess-workspace-pagination .ant-pagination-item-1").click();
  await expect(page.locator(".preprocess-asset-card")).toHaveCount(50);
  await expect(page.locator(".preprocess-asset-card").first()).toHaveCSS("width", "190px");

  await page.locator(".preprocess-import-fab").click();
  await expect(importModal.locator(".preprocess-import-preview-card")).toHaveCount(50);
  await expect(importModal.locator("video")).toHaveCount(0);
});
