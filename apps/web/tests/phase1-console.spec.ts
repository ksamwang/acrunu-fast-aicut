import { expect, test } from "@playwright/test";

test("logs in and renders asset and task status pages", async ({ page }) => {
  await page.route("**/api/**", async (route) => {
    const url = route.request().url();

    if (url.includes("/api/auth/login")) {
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
      return;
    }

    if (url.includes("/api/assets/asset-1/frames")) {
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            asset_id: "asset-1",
            frames: [
              {
                id: "frame-1",
                asset_id: "asset-1",
                frame_index: 0,
                timestamp_ms: 500,
                storage_key: "frames/asset-1/frame_000.jpg",
                created_at: "2026-07-08T00:00:00Z"
              }
            ]
          }
        })
      });
      return;
    }

    if (url.includes("/api/products")) {
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({
          data: [
            {
              id: "product-1",
              name: "Smart Light",
              category: "auto",
              status: "active"
            }
          ]
        })
      });
      return;
    }

    if (url.includes("/api/assets")) {
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({
          data: [
            {
              id: "asset-1",
              product_id: "product-1",
              asset_name: "clean-shot",
              storage_key: "assets/clean-shot.mp4",
              file_name: "clean-shot.mp4",
              source_type: "visual_only",
              status: "ready",
              analysis_status: "ready",
              usability_status: "usable",
              duration_ms: 2066,
              width: 320,
              height: 568,
              has_audio: true,
              audio_codec: "aac",
              bitrate_kbps: 3200,
              scene_description: "product close-up with stable framing",
              shot_size: "close_up",
              camera_movement: "static",
              subjects: ["product"],
              scene_tags: ["indoor", "demo"],
              quality_tags: []
            }
          ]
        })
      });
      return;
    }

    if (url.includes("/api/tasks")) {
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
      return;
    }

    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ data: [] })
    });
  });

  await page.goto("/");
  await expect(page.getByTestId("login-page")).toBeVisible();
  await page.getByTestId("login-submit").click();
  await expect(page.getByTestId("console-app")).toBeVisible();

  await page.locator(".ant-menu-item").nth(1).click();
  await expect(page.getByTestId("assets-page")).toBeVisible();
  await expect(page.getByText("clean-shot.mp4")).toBeVisible();
  await expect(page.getByRole("cell", { name: "ready" }).first()).toBeVisible();
  await expect(page.getByText("Smart Light")).toBeVisible();
  await expect(page.getByText("320x568")).toBeVisible();
  await expect(page.getByText("close_up")).toBeVisible();
  await expect(page.getByText("static")).toBeVisible();
  await expect(page.getByText("indoor, demo")).toBeVisible();
  await page.getByRole("button", { name: "clean-shot" }).click();
  await expect(page.getByTestId("asset-detail-modal")).toBeVisible();
  await expect(page.getByTestId("asset-analysis-panel")).toBeVisible();
  await expect(page.getByText("product close-up with stable framing")).toBeVisible();
  await expect(page.getByText("No quality issues")).toBeVisible();
  await expect(page.getByText("0:00.500")).toBeVisible();
  await expect(page.getByTestId("frame-card")).toBeVisible();
  await page.locator(".ant-modal-close").click();
  await expect(page.getByRole("dialog")).toBeHidden();

  await page.locator(".ant-menu-item").nth(2).click();
  await expect(page.getByTestId("tasks-page")).toBeVisible();
  await expect(page.getByText("task-1")).toBeVisible();
  await expect(page.getByText("completed")).toBeVisible();
});
