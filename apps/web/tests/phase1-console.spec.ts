import { expect, test } from "@playwright/test";

test("logs in and renders asset and task status pages", async ({ page }) => {
  await page.route("**/api/**", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ data: [] })
    });
  });

  await page.route("**/api/auth/login", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        data: {
          token: "test-token",
          user: {
            id: "dev-admin",
            username: "admin",
            display_name: "Admin",
            role: "admin"
          }
        }
      })
    });
  });

  await page.route("**/api/products", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ data: [] })
    });
  });

  await page.route("**/api/assets", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        data: [
          {
            id: "asset-1",
            file_name: "clean-shot.mp4",
            source_type: "visual_only",
            status: "ready",
            duration_ms: 2066,
            width: 320,
            height: 568
          }
        ]
      })
    });
  });

  await page.route("**/api/tasks**", async (route) => {
    if (route.request().method() === "POST") {
      await route.fulfill({
        status: 201,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            id: "task-1",
            task_type: "test",
            status: "queued",
            retry_count: 0,
            created_at: "2026-07-08T00:00:00Z"
          }
        })
      });
      return;
    }

    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        data: [
          {
            id: "task-1",
            task_type: "test",
            status: "completed",
            retry_count: 0,
            created_at: "2026-07-08T00:00:00Z"
          }
        ]
      })
    });
  });

  await page.goto("/");
  await expect(page.getByTestId("login-page")).toBeVisible();
  await page.getByTestId("login-submit").click();
  await expect(page.getByTestId("console-app")).toBeVisible();

  await page.locator(".ant-menu-item").nth(1).click();
  await expect(page.getByTestId("assets-page")).toBeVisible();
  await expect(page.getByText("clean-shot.mp4")).toBeVisible();
  await expect(page.getByText("ready")).toBeVisible();
  await expect(page.getByText("320x568")).toBeVisible();

  await page.locator(".ant-menu-item").nth(2).click();
  await expect(page.getByTestId("tasks-page")).toBeVisible();
  await expect(page.getByText("task-1")).toBeVisible();
  await expect(page.getByText("completed")).toBeVisible();
});
