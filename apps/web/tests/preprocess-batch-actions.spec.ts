import { expect, test, type Page, type Route } from "@playwright/test";

type MockWorkspaceItem = {
  id: string;
  status: "pending" | "saved" | "ready_to_submit" | "submitted";
  product_id?: string;
  asset_name: string;
  source_type: "visual_only";
  use_original_audio: boolean;
  original_file_name: string;
  source_in_ms: number;
  source_out_ms: number;
  transcript: string;
  transcript_segments: never[];
  reviewer_notes: string;
  probe: { duration_ms: number; width: number; height: number; fps: number; has_audio: boolean };
  preview_frame_snapshots: never[];
  frame_snapshots: never[];
  source_url: string;
  thumbnail_url: string;
  vlm_status: "idle" | "queued" | "running" | "ready" | "failed";
  updated_at: string;
};

const transparentPixel = "data:image/gif;base64,R0lGODlhAQABAAD/ACwAAAAAAQABAAACADs=";
const product = {
  id: "product-1",
  name: "束裤带",
  metadata: { reference_image: transparentPixel }
};

function workspaceItem(index: number, status: MockWorkspaceItem["status"] = "pending"): MockWorkspaceItem {
  const id = `workspace-${index}`;
  return {
    id,
    status,
    asset_name: `素材 ${index}`,
    source_type: "visual_only",
    use_original_audio: index % 2 === 0,
    original_file_name: `source-${index}.mp4`,
    source_in_ms: 0,
    source_out_ms: 6000,
    transcript: "",
    transcript_segments: [],
    reviewer_notes: "",
    probe: { duration_ms: 6000, width: 1080, height: 1920, fps: 30, has_audio: true },
    preview_frame_snapshots: [],
    frame_snapshots: [],
    source_url: `http://127.0.0.1:58721/workspace/items/${id}/source`,
    thumbnail_url: transparentPixel,
    vlm_status: "idle",
    updated_at: "2026-07-17T00:00:00.000Z"
  };
}

async function installSessionAndAPIMocks(page: Page) {
  await page.addInitScript(() => {
    window.localStorage.setItem("aicut.session", JSON.stringify({
      token: "preprocess-batch-token",
      user: { id: "admin-1", username: "admin", display_name: "Admin", role: "admin" }
    }));
  });
  await page.route((url) => url.pathname.startsWith("/api/"), async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname === "/api/auth/me") {
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({ data: { id: "admin-1", username: "admin", display_name: "Admin", role: "admin" } })
      });
      return;
    }
    if (url.pathname === "/api/products") {
      await route.fulfill({ contentType: "application/json", body: JSON.stringify({ data: [product] }) });
      return;
    }
    if (url.pathname === "/api/uploads/tokens") {
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({ data: { token: `upload-${Date.now()}`, product_id: product.id } })
      });
      return;
    }
    await route.fulfill({ contentType: "application/json", body: JSON.stringify({ data: [] }) });
  });
}

function listResponse(items: MockWorkspaceItem[]) {
  return {
    items,
    total: items.length,
    page: 1,
    page_size: 50,
    stats: {
      pending: items.filter((item) => item.status === "pending").length,
      saved: items.filter((item) => item.status === "saved").length,
      ready_to_submit: items.filter((item) => item.status === "ready_to_submit").length,
      submitted: items.filter((item) => item.status === "submitted").length
    },
    has_running_vlm: items.some((item) => item.vlm_status === "queued" || item.vlm_status === "running")
  };
}

async function fulfillJSON(route: Route, body: unknown, status = 200) {
  await route.fulfill({
    status,
    contentType: "application/json",
    headers: { "Access-Control-Allow-Origin": "*" },
    body: JSON.stringify(body)
  });
}

test("marquee selects cards and starts batch VLM with the product reference image", async ({ page }) => {
  await installSessionAndAPIMocks(page);
  const items = Array.from({ length: 6 }, (_, index) => workspaceItem(index + 1));
  const vlmPayloads: Array<Record<string, unknown>> = [];

  await page.route("http://127.0.0.1:58721/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    if (request.method() === "OPTIONS") {
      await route.fulfill({ status: 204, headers: { "Access-Control-Allow-Origin": "*" } });
      return;
    }
    if (request.method() === "GET" && url.pathname === "/workspace/items") {
      await fulfillJSON(route, listResponse(items));
      return;
    }
    const itemMatch = url.pathname.match(/^\/workspace\/items\/([^/]+)$/);
    if (request.method() === "GET" && itemMatch) {
      await fulfillJSON(route, { item: items.find((item) => item.id === itemMatch[1]) });
      return;
    }
    const vlmMatch = url.pathname.match(/^\/workspace\/items\/([^/]+)\/vlm-label$/);
    if (request.method() === "POST" && vlmMatch) {
      const payload = request.postDataJSON() as Record<string, unknown>;
      vlmPayloads.push(payload);
      const item = items.find((candidate) => candidate.id === vlmMatch[1])!;
      item.product_id = String(payload.product_id);
      item.vlm_status = "ready";
      await fulfillJSON(route, { item }, 202);
      return;
    }
    await fulfillJSON(route, { error: "not found" }, 404);
  });

  await page.goto("/#/preprocess");
  const cards = page.locator(".preprocess-asset-card");
  await expect(cards).toHaveCount(6);

  await cards.first().click();
  await expect(cards.first()).toHaveAttribute("aria-pressed", "true");
  await expect(page.locator(".preprocess-modal")).not.toBeVisible();
  await cards.first().dblclick();
  await expect(page.locator(".preprocess-modal")).toBeVisible();
  await page.locator(".preprocess-modal .ant-modal-close").click();
  await expect(page.locator(".preprocess-modal")).not.toBeVisible();

  const first = await cards.nth(0).boundingBox();
  const second = await cards.nth(1).boundingBox();
  if (!first || !second) {
    throw new Error("preprocess cards are not visible");
  }
  await page.mouse.move(first.x - 10, first.y - 10);
  await page.mouse.down();
  await page.mouse.move(second.x + second.width - 4, second.y + second.height - 4, { steps: 6 });
  await page.mouse.up();
  await expect(page.getByText("已选 2 项", { exact: true })).toBeVisible();

  await cards.nth(1).click({ button: "right" });
  const contextMenu = page.locator(".ant-dropdown-menu");
  await expect(contextMenu.getByText("批量VLM", { exact: true })).toBeVisible();
  await expect(contextMenu.getByText("正式提交", { exact: true })).toBeVisible();
  await expect(contextMenu.getByText("删除", { exact: true })).toBeVisible();
  await contextMenu.getByText("批量VLM", { exact: true }).click();

  const modal = page.locator(".preprocess-batch-modal").filter({ hasText: "批量 VLM" });
  await modal.getByRole("combobox").click();
  await page.getByText("束裤带", { exact: true }).last().click();
  await expect(modal.getByRole("checkbox", { name: "使用产品参考图" })).toBeChecked();
  await modal.locator(".ant-modal-footer .ant-btn-primary").click();
  await expect.poll(() => vlmPayloads.length).toBe(2);
  expect(vlmPayloads.every((payload) => payload.product_id === product.id)).toBe(true);
  expect(vlmPayloads.every((payload) => payload.product_reference_image_data_url === transparentPixel)).toBe(true);
});

test("formal submit auto-prepares mixed statuses and batch delete removes only local records", async ({ page }) => {
  await installSessionAndAPIMocks(page);
  const items = [
    workspaceItem(1, "pending"),
    workspaceItem(2, "saved"),
    workspaceItem(3, "ready_to_submit"),
    workspaceItem(4, "submitted")
  ];
  const preparedIDs: string[] = [];
  const submittedIDs: string[] = [];
  const deletedIDs: string[] = [];

  await page.route("http://127.0.0.1:58721/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    if (request.method() === "OPTIONS") {
      await route.fulfill({ status: 204, headers: { "Access-Control-Allow-Origin": "*" } });
      return;
    }
    if (request.method() === "GET" && url.pathname === "/workspace/items") {
      await fulfillJSON(route, listResponse(items));
      return;
    }
    const itemMatch = url.pathname.match(/^\/workspace\/items\/([^/]+)$/);
    const item = itemMatch ? items.find((candidate) => candidate.id === itemMatch[1]) : undefined;
    if (request.method() === "GET" && item) {
      await fulfillJSON(route, { item });
      return;
    }
    if (request.method() === "PUT" && item) {
      item.status = "saved";
      await fulfillJSON(route, { item });
      return;
    }
    if (request.method() === "DELETE" && item) {
      deletedIDs.push(item.id);
      items.splice(items.indexOf(item), 1);
      await fulfillJSON(route, { deleted: true });
      return;
    }
    const prepareMatch = url.pathname.match(/^\/workspace\/items\/([^/]+)\/prepare$/);
    if (request.method() === "POST" && prepareMatch) {
      const prepared = items.find((candidate) => candidate.id === prepareMatch[1])!;
      prepared.status = "ready_to_submit";
      preparedIDs.push(prepared.id);
      await fulfillJSON(route, { item: prepared });
      return;
    }
    const submitMatch = url.pathname.match(/^\/workspace\/items\/([^/]+)\/submit$/);
    if (request.method() === "POST" && submitMatch) {
      const submitted = items.find((candidate) => candidate.id === submitMatch[1])!;
      submitted.status = "submitted";
      submitted.product_id = product.id;
      submittedIDs.push(submitted.id);
      await fulfillJSON(route, { item: submitted });
      return;
    }
    await fulfillJSON(route, { error: "not found" }, 404);
  });

  await page.goto("/#/preprocess");
  const cards = page.locator(".preprocess-asset-card");
  await cards.nth(0).click();
  for (let index = 1; index < 4; index += 1) {
    await cards.nth(index).click({ modifiers: ["Control"] });
  }
  await expect(page.getByText("已选 4 项", { exact: true })).toBeVisible();

  await cards.first().click({ button: "right" });
  await page.locator(".ant-dropdown-menu").getByText("正式提交", { exact: true }).click();
  const submitModal = page.locator(".preprocess-batch-modal").filter({ hasText: "正式提交" });
  await expect(submitModal.getByText("自动完成处理")).toBeVisible();
  await submitModal.getByRole("combobox").click();
  await page.getByText("束裤带", { exact: true }).last().click();
  await submitModal.getByRole("button", { name: "确认提交" }).click();
  await expect.poll(() => submittedIDs.length).toBe(3);
  expect(preparedIDs.sort()).toEqual(["workspace-1", "workspace-2"]);

  await cards.first().click({ button: "right" });
  await page.locator(".ant-dropdown-menu").getByText("删除", { exact: true }).click();
  const confirm = page.locator(".ant-modal-confirm");
  await expect(confirm.locator(".ant-modal-confirm-title")).toHaveText("删除 4 项本地素材？");
  await confirm.locator(".ant-btn-primary.ant-btn-dangerous").click();
  await expect.poll(() => deletedIDs.length).toBe(4);
  await expect(page.locator(".preprocess-asset-card")).toHaveCount(0);
});
