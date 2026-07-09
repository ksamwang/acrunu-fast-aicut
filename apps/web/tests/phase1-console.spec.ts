import { expect, test } from "@playwright/test";

test("logs in and renders asset and task status pages", async ({ page }) => {
  let assetSellingPoints = [
    {
      id: "sp-1",
      product_id: "product-1",
      title: "Auto Wake",
      priority: 1,
      status: "active"
    }
  ];

  let asset = {
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
    quality_tags: [] as string[],
    reviewer_notes: "",
    archived_at: undefined as string | undefined
  };
  const filteredAsset = {
    ...asset,
    id: "asset-2",
    asset_name: "mute-shot",
    file_name: "mute-shot.mp4",
    source_type: "talking_head",
    has_audio: false,
    duration_ms: 900,
    scene_description: "speaker intro",
    shot_size: "medium_close_up",
    camera_movement: "handheld",
    subjects: ["speaker"],
    scene_tags: ["talking_head"],
    quality_tags: ["low_light"] as string[]
  };

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

    if (url.includes("/api/assets/asset-1/selling-points")) {
      if (route.request().method() === "PUT") {
        const body = route.request().postDataJSON() as { selling_point_ids?: string[] };
        const allSellingPoints = [
          {
            id: "sp-1",
            product_id: "product-1",
            title: "Auto Wake",
            priority: 1,
            status: "active"
          },
          {
            id: "sp-2",
            product_id: "product-1",
            title: "Battery Saver",
            priority: 2,
            status: "active"
          }
        ];
        assetSellingPoints = allSellingPoints.filter((item) => body.selling_point_ids?.includes(item.id));
      }

      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({
          data: assetSellingPoints
        })
      });
      return;
    }

    if (url.includes("/api/selling-points/sp-1/assets")) {
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({
          data: [asset, filteredAsset]
        })
      });
      return;
    }

    if (url.includes("/api/products")) {
      if (url.includes("/api/products/product-1/stats")) {
        await route.fulfill({
          contentType: "application/json",
          body: JSON.stringify({
            data: {
              product_id: "product-1",
              asset_count: 2,
              usable_asset_count: 1,
              pending_analysis_count: 1
            }
          })
        });
        return;
      }

      if (url.includes("/selling-points")) {
        await route.fulfill({
          contentType: "application/json",
          body: JSON.stringify({
            data: [
              {
                id: "sp-1",
                product_id: "product-1",
                title: "Auto Wake",
                priority: 1,
                status: "active",
                asset_count: 2
              },
              {
                id: "sp-2",
                product_id: "product-1",
                title: "Battery Saver",
                priority: 2,
                status: "active",
                asset_count: 1
              }
            ]
          })
        });
        return;
      }

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
      if (url.includes("/api/assets/asset-1/archive") && route.request().method() === "POST") {
        asset = {
          ...asset,
          status: "archived",
          archived_at: "2026-07-08T00:01:00Z"
        };
        await route.fulfill({
          contentType: "application/json",
          body: JSON.stringify({ data: asset })
        });
        return;
      }

      if (url.includes("/api/assets/asset-1/restore") && route.request().method() === "POST") {
        asset = {
          ...asset,
          status: "ready",
          archived_at: undefined
        };
        await route.fulfill({
          contentType: "application/json",
          body: JSON.stringify({ data: asset })
        });
        return;
      }

      if (url.includes("/api/assets/asset-1/review") && route.request().method() === "PUT") {
        const body = route.request().postDataJSON() as Record<string, unknown>;
        asset = {
          ...asset,
          scene_description: String(body.scene_description ?? ""),
          shot_size: String(body.shot_size ?? ""),
          camera_movement: String(body.camera_movement ?? ""),
          subjects: (body.subjects as string[]) ?? [],
          scene_tags: (body.scene_tags as string[]) ?? [],
          quality_tags: (body.quality_tags as string[]) ?? [],
          usability_status: String(body.usability_status ?? ""),
          reviewer_notes: String(body.reviewer_notes ?? "")
        };
        await route.fulfill({
          contentType: "application/json",
          body: JSON.stringify({ data: asset })
        });
        return;
      }

      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            items:
              url.includes("selling_point_id=sp-1") ||
              url.includes("tag=demo") ||
              url.includes("min_duration_ms=2000") ||
              url.includes("has_audio=true")
                ? [asset]
                : url.includes("has_audio=false")
                  ? [filteredAsset]
                  : [asset, filteredAsset],
            total:
              url.includes("selling_point_id=sp-1") ||
              url.includes("tag=demo") ||
              url.includes("min_duration_ms=2000") ||
              url.includes("has_audio=true") ||
              url.includes("has_audio=false")
                ? 1
                : 2,
            page: 1,
            page_size: 20
          }
        })
      });
      return;
    }

    if (url.includes("/api/tasks")) {
      if (url.includes("/api/tasks/task-asset-analyze")) {
        await route.fulfill({
          contentType: "application/json",
          body: JSON.stringify({
            data: {
              id: "task-asset-analyze",
              task_type: "asset_analyze",
              status: "failed",
              asset_id: "asset-1",
              payload_summary: {
                asset_id: "asset-1",
                storage_key: "assets/clean-shot.mp4"
              },
              error_message: "mock provider failed",
              retry_count: 1,
              duration_ms: 231,
              created_at: "2026-07-08T00:00:00Z",
              started_at: "2026-07-08T00:00:01Z",
              finished_at: "2026-07-08T00:00:01Z"
            }
          })
        });
        return;
      }

      if (url.includes("/api/tasks/task-1")) {
        await route.fulfill({
          contentType: "application/json",
          body: JSON.stringify({
            data: {
              id: "task-1",
              task_type: "test",
              status: "completed",
              payload_summary: {
                kind: "test"
              },
              retry_count: 0,
              duration_ms: 100,
              created_at: "2026-07-08T00:00:00Z",
              started_at: "2026-07-08T00:00:00Z",
              finished_at: "2026-07-08T00:00:00Z"
            }
          })
        });
        return;
      }

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
              duration_ms: 100,
              created_at: "2026-07-08T00:00:00Z"
            },
            {
              id: "task-asset-analyze",
              task_type: "asset_analyze",
              status: "failed",
              asset_id: "asset-1",
              retry_count: 1,
              duration_ms: 231,
              error_message: "mock provider failed",
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

  await expect(page.getByText("Smart Light")).toBeVisible();
  await page.getByRole("cell", { name: "Smart Light" }).click();
  await expect(page.getByText("Pending Analysis")).toBeVisible();
  await expect(page.getByTestId("product-asset-count")).toHaveText("2");
  await expect(page.getByTestId("product-usable-asset-count")).toHaveText("1");
  await expect(page.getByTestId("product-pending-analysis-count")).toHaveText("1");
  await page.getByRole("cell", { name: "Auto Wake" }).click();
  await expect(page.getByText("Selling Point Assets")).toBeVisible();
  await expect(page.getByText("clean-shot")).toBeVisible();
  await expect(page.getByText("mute-shot")).toBeVisible();

  await page.locator(".ant-menu-item").nth(1).click();
  await expect(page.getByTestId("assets-page")).toBeVisible();
  await expect(page.getByText("clean-shot.mp4")).toBeVisible();
  await expect(page.getByRole("cell", { name: "ready" }).first()).toBeVisible();
  await expect(page.getByRole("cell", { name: "Smart Light" }).first()).toBeVisible();
  await expect(page.getByRole("cell", { name: "320x568" }).first()).toBeVisible();
  await expect(page.getByRole("cell", { name: "close_up" }).first()).toBeVisible();
  await expect(page.getByRole("cell", { name: "static" }).first()).toBeVisible();
  await expect(page.getByText("indoor, demo")).toBeVisible();
  await expect(page.getByText("mute-shot.mp4")).toBeVisible();
  await page.getByTestId("asset-filter-product").click();
  await page.getByTitle("Smart Light").click();
  await page.getByTestId("asset-filter-selling-point").click();
  await page.getByTitle("Auto Wake").click();
  await page.getByTestId("asset-filter-tag").fill("demo");
  await page.getByTestId("asset-filter-min-duration").fill("2000");
  await page.getByTestId("asset-filter-has-audio").click();
  await page.getByTitle("audio only").click();
  await expect(page.getByText("clean-shot.mp4")).toBeVisible();
  await expect(page.getByText("mute-shot.mp4")).toHaveCount(0);
  await page.getByRole("button", { name: "clean-shot" }).click();
  await expect(page.getByTestId("asset-detail-modal")).toBeVisible();
  await expect(page.getByTestId("asset-analysis-panel")).toBeVisible();
  await expect(page.getByText("product close-up with stable framing")).toBeVisible();
  await expect(page.getByText("No quality issues")).toBeVisible();
  await expect(page.locator(".ant-tag").filter({ hasText: "Auto Wake" }).last()).toBeVisible();
  await expect(page.getByText("0:00.500")).toBeVisible();
  await expect(page.getByTestId("frame-card")).toBeVisible();
  await page.getByRole("button", { name: "Edit Tags" }).click();
  await expect(page.getByTestId("asset-review-form")).toBeVisible();
  await page.getByLabel("Scene Description").fill("manual revised description");
  await page.getByLabel("Reviewer Notes").fill("reviewed by editor");
  await page.getByLabel("Quality Tags").fill("soft_focus");
  await page.keyboard.press("Enter");
  await page.getByTestId("save-asset-review").click();
  await expect(page.getByText("manual revised description")).toBeVisible();
  await expect(page.getByText("reviewed by editor")).toBeVisible();
  await expect(page.getByText("soft_focus")).toBeVisible();
  await expect(page.getByText("None")).toBeVisible();
  await page.getByTestId("asset-selling-points-select").click();
  await page.locator(".ant-select-dropdown:visible").getByTitle("Battery Saver").click();
  await page.getByTestId("save-asset-selling-points").click();
  await expect(page.locator(".ant-tag").filter({ hasText: "Battery Saver" }).last()).toBeVisible();
  await page.getByTestId("archive-asset").click();
  await expect(page.getByText("archived")).toBeVisible();
  await page.getByTestId("restore-asset").click();
  await expect(page.getByTestId("archive-asset")).toBeVisible();
  await page.locator(".ant-modal-close").click();
  await expect(page.getByRole("dialog")).toBeHidden();

  await page.locator(".ant-menu-item").nth(2).click();
  await expect(page.getByTestId("tasks-page")).toBeVisible();
  await expect(page.getByText("task-1")).toBeVisible();
  await expect(page.getByText("completed")).toBeVisible();
  await expect(page.getByText("asset_analyze")).toBeVisible();
  await expect(page.getByText("231 ms")).toBeVisible();
  await page.getByRole("button", { name: "task-asset-analyze" }).click();
  await expect(page.getByTestId("task-detail-modal")).toBeVisible();
  await expect(page.getByText("mock provider failed")).toBeVisible();
  await expect(page.getByText("\"asset_id\": \"asset-1\"")).toBeVisible();
});
