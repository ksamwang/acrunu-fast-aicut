import { expect, test, type Page, type Route } from "@playwright/test";

const user = {
  id: "user-1",
  username: "test",
  display_name: "测试用户",
  role: "user"
};

function createWorks() {
  return Array.from({ length: 14 }, (_, index) => {
    const number = index + 1;
    const failed = number === 6;
    return {
      id: `work-${number}`,
      run_id: `work-${number}`,
      product_id: "product-1",
      product_name: "骑行车包",
      created_by_name: "测试用户",
      title: `成品 ${number}`,
      hook: `成品 ${number}`,
      script_text: `这是第 ${number} 条成品文案。`,
      duration_ms: 10_000,
      status: failed ? "failed" : "completed",
      progress: 100,
      stage_label: failed ? "生成失败" : "已完成",
      error_message: failed ? "测试失败" : undefined,
      created_at: `2026-07-30T${String(23 - index).padStart(2, "0")}:00:00.000Z`,
      completed_at: failed ? undefined : `2026-07-30T${String(23 - index).padStart(2, "0")}:01:00.000Z`,
      video_url: failed ? undefined : `/storage/work-${number}.mp4`
    };
  });
}

async function fulfillJSON(route: Route, data: unknown) {
  await route.fulfill({
    contentType: "application/json",
    body: JSON.stringify({ data })
  });
}

async function installMocks(page: Page) {
  const works = createWorks();
  await page.addInitScript((session) => {
    window.localStorage.setItem("aicut.session", JSON.stringify(session));
  }, { token: "finished-navigation-token", user });

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
      await fulfillJSON(route, [{ id: "product-1", name: "骑行车包", status: "active" }]);
      return;
    }
    if (url.pathname === "/api/workbench/works") {
      await fulfillJSON(route, works);
      return;
    }

    const actionMatch = url.pathname.match(/^\/api\/workbench\/works\/(work-\d+)\/(retry|regenerate)$/);
    if (actionMatch && route.request().method() === "POST") {
      const work = works.find((item) => item.id === actionMatch[1]);
      if (!work) {
        await route.fulfill({ status: 404 });
        return;
      }
      Object.assign(work, {
        status: "generating",
        progress: 5,
        stage_label: actionMatch[2] === "retry" ? "准备重试" : "准备重新生成",
        error_message: undefined,
        completed_at: undefined,
        video_url: undefined
      });
      await fulfillJSON(route, work);
      return;
    }

    const deleteMatch = url.pathname.match(/^\/api\/workbench\/works\/(work-\d+)$/);
    if (deleteMatch && route.request().method() === "DELETE") {
      const index = works.findIndex((item) => item.id === deleteMatch[1]);
      if (index < 0) {
        await route.fulfill({ status: 404 });
        return;
      }
      works.splice(index, 1);
      await fulfillJSON(route, { deleted: true });
      return;
    }

    await fulfillJSON(route, []);
  });
}

test("switches finished works with the wheel and restores the current card", async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 720 });
  await installMocks(page);
  await page.goto("/#/finished");

  const originalCard = page.getByTestId("finished-work-work-9");
  await originalCard.scrollIntoViewIfNeeded();
  await originalCard.getByRole("button", { name: "查看 成品 9" }).click();

  const detail = page.getByTestId("finished-work-detail");
  await expect(detail).toBeVisible();
  await expect(detail.locator(".finished-detail-title")).toHaveText("成品 9");
  await expect(detail.locator(".finished-detail-preview-position")).toHaveText("9/14");

  await detail.locator(".finished-detail-preview").dispatchEvent("wheel", { deltaY: 120, deltaX: 0, deltaMode: 0 });
  await expect(detail.locator(".finished-detail-title")).toHaveText("成品 10");
  await expect(detail.locator(".finished-detail-preview-position")).toHaveText("10/14");

  await detail.getByRole("button", { name: "返回成品库" }).click();
  await expect(page.getByTestId("finished-library-page")).toBeVisible();
  await expect(page.getByTestId("finished-work-work-10")).toBeInViewport();
  await expect.poll(() => page.locator(".finished-work-scroll").evaluate((element) => element.scrollTop)).toBeGreaterThan(0);

  await page.reload();
  await expect(page.getByTestId("finished-library-page")).toBeVisible();
  await expect(page.getByTestId("finished-work-work-10")).toBeInViewport();
});

test("deletes the current work and retries a failed work from the detail page", async ({ page }) => {
  await installMocks(page);
  await page.goto("/#/finished");

  await page.getByTestId("finished-work-work-3").getByRole("button", { name: "查看 成品 3" }).click();
  const detail = page.getByTestId("finished-work-detail");
  await expect(detail.getByRole("button", { name: "重新生成" })).toBeEnabled();
  await detail.getByRole("button", { name: "删除", exact: true }).click();
  const deleteDialog = page.getByRole("dialog");
  await expect(deleteDialog).toContainText("删除成片？");
  await deleteDialog.getByRole("button", { name: /删\s*除/ }).click();
  await expect(detail.locator(".finished-detail-title")).toHaveText("成品 4");
  await expect(page).toHaveURL(/#\/finished\/work-4$/);

  await detail.getByRole("button", { name: "返回成品库" }).click();
  await page.getByTestId("finished-work-work-6").getByRole("button", { name: "查看 成品 6" }).click();
  await expect(detail.getByRole("button", { name: "重试" })).toBeEnabled();
  await detail.getByRole("button", { name: "重试" }).click();
  await expect(detail.getByText("生成中", { exact: true })).toBeVisible();
  await expect(detail.getByRole("button", { name: "重新生成" })).toBeDisabled();
  await expect(detail.getByRole("button", { name: "删除", exact: true })).toBeDisabled();
});
