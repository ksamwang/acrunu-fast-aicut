import { expect, test } from "@playwright/test";

const user = {
  id: "user-1",
  username: "test",
  display_name: "测试用户",
  role: "user"
};

const works = [
  {
    id: "work-newer",
    run_id: "work-newer",
    product_id: "product-1",
    product_name: "骑行车包",
    created_by_name: "测试用户",
    title: "较新的成片",
    hook: "较新的成片",
    script_text: "按创建时间倒序排列。",
    duration_ms: 10_000,
    status: "completed",
    progress: 100,
    stage_label: "已完成",
    created_at: "2026-07-15T10:00:00.000Z",
    completed_at: "2026-07-15T10:01:00.000Z"
  },
  {
    id: "work-older-date",
    run_id: "work-older-date",
    product_id: "product-1",
    product_name: "骑行车包",
    created_by_name: "测试用户",
    title: "前一日期成片",
    hook: "前一日期成片",
    script_text: "属于前一日期。",
    duration_ms: 10_000,
    status: "failed",
    progress: 100,
    stage_label: "生成失败",
    error_message: "测试失败",
    created_at: "2026-07-14T08:00:00.000Z"
  },
  {
    id: "work-generating",
    run_id: "work-generating",
    product_id: "product-1",
    product_name: "骑行车包",
    created_by_name: "测试用户",
    title: "生成中的成片",
    hook: "生成中的成片",
    script_text: "生成中的项目不可批量选择。",
    duration_ms: 0,
    status: "generating",
    progress: 50,
    stage_label: "生成编排",
    created_at: "2026-07-15T09:00:00.000Z"
  },
  {
    id: "work-older",
    run_id: "work-older",
    product_id: "product-1",
    product_name: "骑行车包",
    created_by_name: "测试用户",
    title: "较早的成片",
    hook: "较早的成片",
    script_text: "同一天内排在后面。",
    duration_ms: 10_000,
    status: "completed",
    progress: 100,
    stage_label: "已完成",
    created_at: "2026-07-15T08:00:00.000Z",
    completed_at: "2026-07-15T08:01:00.000Z"
  }
];

test("groups finished works by creation date and selects one date", async ({ page }) => {
  await page.addInitScript((session) => {
    window.localStorage.setItem("aicut.session", JSON.stringify(session));
  }, { token: "test-token", user });

  await page.route((url) => url.pathname.startsWith("/api/"), async (route) => {
    const requestURL = new URL(route.request().url());
    if (requestURL.pathname === "/api/auth/login") {
      await route.fulfill({ contentType: "application/json", body: JSON.stringify({ data: { token: "test-token", user } }) });
      return;
    }
    if (requestURL.pathname === "/api/auth/me") {
      await route.fulfill({ contentType: "application/json", body: JSON.stringify({ data: user }) });
      return;
    }
    if (requestURL.pathname === "/api/products") {
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({ data: [{ id: "product-1", name: "骑行车包", status: "active", created_at: "2026-07-01T00:00:00.000Z", updated_at: "2026-07-01T00:00:00.000Z" }] })
      });
      return;
    }
    if (requestURL.pathname === "/api/workbench/works") {
      await route.fulfill({ contentType: "application/json", body: JSON.stringify({ data: works }) });
      return;
    }
    await route.fulfill({ contentType: "application/json", body: JSON.stringify({ data: [] }) });
  });

  await page.goto("/#/finished");
  await expect(page.getByTestId("console-app")).toBeVisible();
  await expect(page.getByTestId("finished-library-page")).toBeVisible();

  const dateGroups = page.locator("[data-testid^='finished-date-group-']");
  await expect(dateGroups).toHaveCount(2);
  await expect(dateGroups.nth(0)).toHaveAttribute("data-testid", "finished-date-group-2026-07-15");
  await expect(dateGroups.nth(1)).toHaveAttribute("data-testid", "finished-date-group-2026-07-14");

  const currentGroup = page.getByTestId("finished-date-group-2026-07-15");
  const previousGroup = page.getByTestId("finished-date-group-2026-07-14");
  await expect(currentGroup).toContainText("7月15日");
  await expect(currentGroup).toContainText("3 条");
  await expect(previousGroup).toContainText("7月14日");
  await expect(previousGroup).toContainText("1 条");

  const currentCards = currentGroup.locator("[data-testid^='finished-work-']");
  await expect(currentCards.nth(0)).toHaveAttribute("data-testid", "finished-work-work-newer");
  await expect(currentCards.nth(1)).toHaveAttribute("data-testid", "finished-work-work-generating");
  await expect(currentCards.nth(2)).toHaveAttribute("data-testid", "finished-work-work-older");

  await page.getByRole("button", { name: "批量选择" }).click();
  await currentGroup.getByText("选择该日", { exact: true }).click();
  await expect(page.getByText("已选 2 项", { exact: true })).toBeVisible();
  await expect(currentGroup.getByRole("checkbox", { name: "选择 较新的成片", exact: true })).toBeChecked();
  await expect(currentGroup.getByRole("checkbox", { name: "选择 较早的成片", exact: true })).toBeChecked();
  await expect(currentGroup.getByRole("checkbox", { name: "选择 生成中的成片", exact: true })).toBeDisabled();

  await previousGroup.getByText("选择该日", { exact: true }).click();
  await expect(page.getByText("已选 3 项", { exact: true })).toBeVisible();
  await currentGroup.getByText("选择该日", { exact: true }).click();
  await expect(page.getByText("已选 1 项", { exact: true })).toBeVisible();
});
