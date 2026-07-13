import { expect, test } from "@playwright/test";

test("logs in and synchronizes Hash routes for core pages", async ({ page }) => {
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
    updated_at: "2026-07-08T00:00:01Z",
    analyzed_at: "2026-07-08T00:00:01Z",
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
    usability_status: "discarded",
    subjects: ["speaker"],
    scene_tags: ["talking_head"],
    quality_tags: ["low_light"] as string[],
    updated_at: "2026-07-08T00:00:02Z",
    analyzed_at: "2026-07-08T00:00:02Z"
  };

  await page.route((url) => url.pathname.startsWith("/api/"), async (route) => {
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
          data: (() => {
            let items =
              url.includes("selling_point_id=sp-1") ||
              url.includes("tag=demo") ||
              url.includes("min_duration_ms=2000") ||
              url.includes("has_audio=true")
                ? [asset]
                : url.includes("has_audio=false")
                  ? [filteredAsset]
                  : [asset, filteredAsset];

            if (url.includes("keyword=stable")) {
              items = [asset];
            }
            if (url.includes("exclude_discarded=true")) {
              items = items.filter((item) => item.usability_status !== "discarded");
            }
            if (url.includes("sort_by=updated_at_desc")) {
              items = [...items].sort((left, right) => String(right.updated_at).localeCompare(String(left.updated_at)));
            }
            if (url.includes("sort_by=analyzed_at_desc")) {
              items = [...items].sort((left, right) => String(right.analyzed_at).localeCompare(String(left.analyzed_at)));
            }

            return {
              items,
              total: items.length,
              page: 1,
              page_size: 20
            };
          })()
        })
      });
      return;
    }

    if (url.includes("/api/tasks")) {
      if (url.includes("/api/tasks/task-extract-1")) {
        await route.fulfill({
          contentType: "application/json",
          body: JSON.stringify({
            data: {
              id: "task-extract-1",
              task_type: "asset_extract_frames",
              status: "completed",
              asset_id: "asset-1",
              payload_summary: {
                asset_id: "asset-1",
                storage_key: "assets/clean-shot.mp4",
                duration_ms: 2066
              },
              retry_count: 0,
              duration_ms: 180,
              created_at: "2026-07-08T00:00:00Z",
              started_at: "2026-07-08T00:00:00Z",
              finished_at: "2026-07-08T00:00:00Z"
            }
          })
        });
        return;
      }

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

      const allTasks = [
        {
          id: "task-1",
          task_type: "test",
          status: "completed",
          retry_count: 0,
          duration_ms: 100,
          created_at: "2026-07-08T00:00:00Z"
        },
        {
          id: "task-extract-1",
          task_type: "asset_extract_frames",
          status: "completed",
          asset_id: "asset-1",
          retry_count: 0,
          duration_ms: 180,
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
      ];
      let tasks = allTasks;
      if (url.includes("task_type=asset_extract_frames")) {
        tasks = tasks.filter((task) => task.task_type === "asset_extract_frames");
      }
      if (url.includes("task_type=asset_analyze")) {
        tasks = tasks.filter((task) => task.task_type === "asset_analyze");
      }
      if (url.includes("status=failed")) {
        tasks = tasks.filter((task) => task.status === "failed");
      }
      if (url.includes("status=completed")) {
        tasks = tasks.filter((task) => task.status === "completed");
      }
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({ data: tasks })
      });
      return;
    }

    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({ data: [] })
    });
  });

  await page.goto("/");
  await page.evaluate(() => window.localStorage.clear());
  await page.reload();
  await expect(page.getByTestId("login-page")).toBeVisible();
  await page.getByTestId("login-submit").click();
  await expect(page.getByTestId("console-app")).toBeVisible();

  await expect(page.getByText("Smart Light")).toBeVisible();
  await page.getByRole("menuitem", { name: "素材" }).click();
  await expect(page.getByTestId("assets-page")).toBeVisible();
  await expect(page.getByText("clean-shot.mp4")).toBeVisible();
  await expect(page.getByText("mute-shot.mp4")).toBeVisible();

  await page.evaluate(() => {
    window.location.hash = "#/tasks";
  });
  await expect(page.getByTestId("tasks-page")).toBeVisible();
  await page.reload();
  await expect(page.getByTestId("tasks-page")).toBeVisible();
  await expect(page.getByText("task-1")).toBeVisible();
  await expect(page.getByText("task-extract-1")).toBeVisible();
  await expect(page.getByText("task-asset-analyze")).toBeVisible();
  await page.getByRole("button", { name: "task-asset-analyze" }).click();
  await expect(page.getByTestId("task-detail-modal")).toBeVisible();
  await expect(page.getByText("mock provider failed")).toBeVisible();
  await expect(page.getByText("\"asset_id\": \"asset-1\"")).toBeVisible();
});
