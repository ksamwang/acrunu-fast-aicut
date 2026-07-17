import { expect, test } from "@playwright/test";

test("uses the workbench and finished library through Hash routes", async ({ page }) => {
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
  const voiceProfiles = [
    {
      id: "voice-warm-female",
      name: "温和女声",
      language: "中文",
      style_tags: ["自然", "亲和"],
      reference_text: "希望每一次表达都听起来自然、清晰而有温度。",
      preview_text: "这是一段用于确认旁白语速、语气和听感的样音。",
      preview_audio_url: "/storage/voice-profiles/voice-warm-female/preview.wav",
      reference_audio_name: "warm-female.wav",
      preview_status: "ready",
      status: "enabled",
      is_default: true,
      created_at: "2026-07-15T00:00:00.000Z",
      updated_at: "2026-07-15T00:00:00.000Z"
    },
    {
      id: "voice-clear-male",
      name: "清晰男声",
      language: "中文",
      style_tags: ["沉稳", "清晰"],
      reference_text: "用清晰、克制的语气讲清楚每一个重点。",
      preview_text: "这是一段用于确认旁白语速、语气和听感的样音。",
      preview_audio_url: "/storage/voice-profiles/voice-clear-male/preview.wav",
      reference_audio_name: "clear-male.wav",
      preview_status: "ready",
      status: "enabled",
      is_default: false,
      created_at: "2026-07-15T00:00:00.000Z",
      updated_at: "2026-07-15T00:00:00.000Z"
    },
    {
      id: "voice-bright-female",
      name: "明快女声",
      language: "中文",
      style_tags: ["轻快", "有活力"],
      reference_text: "用轻快的节奏带出产品使用时的积极感受。",
      preview_text: "这是一段用于确认旁白语速、语气和听感的样音。",
      preview_audio_url: "/storage/voice-profiles/voice-bright-female/preview.wav",
      reference_audio_name: "bright-female.wav",
      preview_status: "ready",
      status: "enabled",
      is_default: false,
      created_at: "2026-07-15T00:00:00.000Z",
      updated_at: "2026-07-15T00:00:00.000Z"
    }
  ];
  const subtitlePresets = [{
    id: "subtitle-default",
    name: "信息流白字",
    font_family: "Noto Sans CJK SC",
    font_weight: 700,
    text_color: "#FFFFFF",
    background_color: "#000000",
    background_opacity: 0.3,
    outline_color: "#000000",
    outline_width: 0,
    shadow: false,
    max_lines: 2,
    layouts: {
      "9:16": { width: 1080, height: 1920, fps: 30, vertical_position: "center", text_align: "center", vertical_offset_ratio: 0, vertical_position_ratio: 0.82, max_width_ratio: 0.84, font_size_ratio: 0.054, max_chars_per_line: 16 },
      "3:4": { width: 1080, height: 1440, fps: 30, vertical_position: "center", text_align: "center", vertical_offset_ratio: 0, vertical_position_ratio: 0.84, max_width_ratio: 0.88, font_size_ratio: 0.052, max_chars_per_line: 18 }
    },
    status: "enabled",
    is_default: true,
    version: 1,
    created_at: "2026-07-16T00:00:00.000Z",
    updated_at: "2026-07-16T00:00:00.000Z"
  }];
  let savedSubtitlePresetPayload: Record<string, any> | null = null;
  let voiceoverTaskPayload: Record<string, any> | null = null;
  let bgmTracks = [
    {
      id: "bgm-light-1", name: "轻快骑行", file_name: "light-ride.mp3", audio_url: "/storage/bgm/bgm-light-1/source.mp3",
      mime_type: "audio/mpeg", file_size_bytes: 2_400_000, duration_ms: 95_000, sample_rate: 48_000, channels: 2,
      bpm: 118, mood: "轻快", tags: ["骑行", "活力"], status: "enabled",
      created_at: "2026-07-16T00:00:00.000Z", updated_at: "2026-07-16T00:00:00.000Z"
    },
    {
      id: "bgm-warm-1", name: "温暖叙事", file_name: "warm-story.wav", audio_url: "/storage/bgm/bgm-warm-1/source.wav",
      mime_type: "audio/wav", file_size_bytes: 5_800_000, duration_ms: 120_000, sample_rate: 48_000, channels: 2,
      bpm: 92, mood: "温暖", tags: ["叙事"], status: "enabled",
      created_at: "2026-07-16T00:00:00.000Z", updated_at: "2026-07-16T00:00:00.000Z"
    }
  ];
  let generatedWorkPolls = 0;
  let finishedWorks = [
    {
      id: "work-completed-1",
      run_id: "work-completed-1",
      product_id: "product-1",
      product_name: "Smart Light",
      title: "灯光自动唤醒，夜间更安心",
      hook: "回到家，灯光自动亮起。",
      voice_profile_id: "voice-warm-female",
      voice_profile_name: "温和女声",
      script_text: "回到家，灯光自动亮起。无需摸黑找开关，夜间使用更安心。",
      duration_ms: 8500,
      status: "completed",
      progress: 100,
      stage_label: "已完成",
      created_at: "2026-07-15T08:40:00.000Z",
      completed_at: "2026-07-15T08:42:00.000Z",
      editing_intent: "从回家摸黑的场景切入，再展示自动亮灯的结果。",
      bgm: { track_id: "bgm-warm-1", name: "温暖叙事", gain_db: -12 },
      narration_segments: [
        { id: "segment-1", start_ms: 0, end_ms: 4000, text: "回到家，灯光自动亮起。" },
        { id: "segment-2", start_ms: 4000, end_ms: 8500, text: "无需摸黑找开关，夜间使用更安心。" }
      ],
      beats: [
        { id: "beat-1", label: "开头", selling_point: "自动唤醒", visual_goal: "以回家场景建立需求。", source_type: "mixed" },
        { id: "beat-2", label: "展示", selling_point: "夜间安心", visual_goal: "展示自动亮灯和使用结果。", source_type: "visual_only" }
      ],
      audio_url: "/storage/voiceovers/work-completed-1.wav",
      video_url: "/storage/renders/generations/work-completed-1/final.mp4"
    },
    {
      id: "work-delete-1",
      run_id: "work-delete-1",
      product_id: "product-1",
      product_name: "Smart Light",
      title: "用于删除测试的成片",
      hook: "删除测试",
      script_text: "这是一条可以删除的成片。",
      duration_ms: 5000,
      status: "completed",
      progress: 100,
      stage_label: "已完成",
      created_at: "2026-07-15T08:30:00.000Z",
      completed_at: "2026-07-15T08:31:00.000Z",
      video_url: "/storage/renders/generations/work-delete-1/final.mp4"
    }
  ];

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

    if (url.includes("/api/auth/me")) {
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            id: "dev-admin",
            username: "admin",
            display_name: "Admin",
            role: "admin"
          }
        })
      });
      return;
    }

    if (url.includes("/api/admin/users")) {
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({
          data: [
            {
              id: "dev-admin",
              username: "admin",
              display_name: "Admin",
              role: "admin",
              status: "active",
              last_login_at: "2026-07-16T00:00:00.000Z",
              created_at: "2026-07-01T00:00:00.000Z",
              updated_at: "2026-07-16T00:00:00.000Z"
            }
          ]
        })
      });
      return;
    }

    if (url.includes("/api/bgm-tracks")) {
      const method = route.request().method();
      const trackID = new URL(url).pathname.split("/").at(-1);
      if (method === "POST") {
        const created = {
          id: "bgm-uploaded-1", name: "新音乐", file_name: "new-music.mp3", audio_url: "/storage/bgm/bgm-uploaded-1/source.mp3",
          mime_type: "audio/mpeg", file_size_bytes: 1024, duration_ms: 60_000, sample_rate: 48_000, channels: 2,
          bpm: 0, mood: "", tags: [], status: "enabled", created_at: new Date().toISOString(), updated_at: new Date().toISOString()
        };
        bgmTracks = [created, ...bgmTracks];
        await route.fulfill({ status: 201, contentType: "application/json", body: JSON.stringify({ data: created }) });
        return;
      }
      if (method === "PUT") {
        const body = route.request().postDataJSON() as Record<string, any>;
        let updated = bgmTracks.find((track) => track.id === trackID)!;
        updated = { ...updated, ...body, updated_at: new Date().toISOString() };
        bgmTracks = bgmTracks.map((track) => track.id === trackID ? updated : track);
        await route.fulfill({ contentType: "application/json", body: JSON.stringify({ data: updated }) });
        return;
      }
      if (method === "DELETE") {
        let archived = bgmTracks.find((track) => track.id === trackID)!;
        archived = { ...archived, status: "archived" };
        bgmTracks = bgmTracks.map((track) => track.id === trackID ? archived : track);
        await route.fulfill({ contentType: "application/json", body: JSON.stringify({ data: archived }) });
        return;
      }
      const includeInactive = url.includes("include_inactive=true");
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({ data: includeInactive ? bgmTracks : bgmTracks.filter((track) => track.status === "enabled") })
      });
      return;
    }

    if (url.includes("/api/admin/voice-profiles")) {
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({ data: voiceProfiles })
      });
      return;
    }

    if (url.includes("/api/admin/subtitle-presets")) {
      if (route.request().method() === "PUT") {
        savedSubtitlePresetPayload = route.request().postDataJSON() as Record<string, any>;
        subtitlePresets[0] = {
          ...subtitlePresets[0],
          ...savedSubtitlePresetPayload,
          version: subtitlePresets[0].version + 1,
          updated_at: "2026-07-16T00:01:00.000Z"
        };
        await route.fulfill({
          contentType: "application/json",
          body: JSON.stringify({ data: subtitlePresets[0] })
        });
        return;
      }
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({ data: subtitlePresets })
      });
      return;
    }

    if (url.includes("/api/voice-profiles/") && url.includes("/auditions")) {
      await route.fulfill({
        status: 201,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            id: "audition-1",
            task_id: "audition-task-1",
            voice_profile_id: "voice-clear-male",
            voice_profile_name: "清晰男声",
            text: (route.request().postDataJSON() as { text: string }).text,
            audio_url: "/storage/voice-auditions/audition-1.wav",
            sample_rate: 24000,
            duration_ms: 6400,
            status: "completed",
            created_at: "2026-07-15T09:00:00.000Z",
            updated_at: "2026-07-15T09:00:01.000Z"
          }
        })
      });
      return;
    }

    if (url.includes("/api/voice-profiles")) {
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({ data: voiceProfiles })
      });
      return;
    }

    if (url.includes("/api/subtitle-presets")) {
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({ data: subtitlePresets })
      });
      return;
    }

    if (url.includes("/api/workbench/scripts/generate")) {
      const body = route.request().postDataJSON() as { variant_count: number };
      const variants = [
        {
          id: "script-1",
          order: 1,
          hook: "回到家，灯光自动亮起。",
          script_text: "回到家，灯光自动亮起。无需摸黑找开关，自动唤醒让每一次归家都更安心。",
          estimated_duration_ms: 9500,
          editing_intent: "从昏暗归家场景切入，展示自动亮灯和安心使用的结果。",
          beats: [
            { id: "script-1-beat-1", label: "痛点", selling_point: "Auto Wake", visual_goal: "以昏暗归家场景建立需求。", source_type: "mixed" },
            { id: "script-1-beat-2", label: "触发", selling_point: "Auto Wake", visual_goal: "展示设备自动亮起的瞬间。", source_type: "visual_only" },
            { id: "script-1-beat-3", label: "结果", selling_point: "Auto Wake", visual_goal: "展示夜间使用时的安心感。", source_type: "talking_head" }
          ],
          status: "draft",
          updated_at: "2026-07-15T09:05:00.000Z"
        },
        {
          id: "script-2",
          order: 2,
          hook: "不用等你开口，灯光已经准备好了。",
          script_text: "不用等你开口，灯光已经准备好了。自动唤醒减少夜间摸索，让回家这件小事变得更从容。",
          estimated_duration_ms: 10200,
          editing_intent: "以主动服务的视角开场，再强调夜间归家的便利。",
          beats: [
            { id: "script-2-beat-1", label: "开场", selling_point: "Auto Wake", visual_goal: "以门口进入室内的动作开场。", source_type: "mixed" },
            { id: "script-2-beat-2", label: "展示", selling_point: "Auto Wake", visual_goal: "展示自动亮灯过程。", source_type: "visual_only" },
            { id: "script-2-beat-3", label: "收束", selling_point: "Auto Wake", visual_goal: "以舒适稳定的夜间环境收束。", source_type: "talking_head" }
          ],
          status: "draft",
          updated_at: "2026-07-15T09:05:00.000Z"
        },
        {
          id: "script-3",
          order: 3,
          hook: "晚一步开灯，就多一分不方便。",
          script_text: "晚一步开灯，就多一分不方便。自动唤醒在你到家时及时点亮，让夜间动线更清楚、更安心。",
          estimated_duration_ms: 10100,
          editing_intent: "通过夜间行动的不便建立张力，随后用自动亮灯完成问题解决。",
          beats: [
            { id: "script-3-beat-1", label: "冲突", selling_point: "Auto Wake", visual_goal: "用昏暗环境表现行动不便。", source_type: "talking_head" },
            { id: "script-3-beat-2", label: "解决", selling_point: "Auto Wake", visual_goal: "展示灯光自动亮起。", source_type: "visual_only" },
            { id: "script-3-beat-3", label: "结果", selling_point: "Auto Wake", visual_goal: "展示明亮动线带来的安心感。", source_type: "mixed" }
          ],
          status: "draft",
          updated_at: "2026-07-15T09:05:00.000Z"
        }
      ];
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({ data: variants.slice(0, body.variant_count) })
      });
      return;
    }

    if (url.includes("/api/workbench/voiceover-tasks")) {
      const body = route.request().postDataJSON() as {
        product_id: string;
        voice_profile_id: string;
        output_ratio: "9:16" | "3:4";
        subtitle_preset_id: string;
        variants: Array<{ hook: string; script_text: string; editing_intent: string; beats: unknown[]; bgm: { mode: string; track_id: string; gain_db: number } }>;
      };
      voiceoverTaskPayload = body;
      const profile = voiceProfiles.find((item) => item.id === body.voice_profile_id) ?? voiceProfiles[0];
      const createdWorks = body.variants.map((variant, index) => ({
        id: `work-generated-${index + 1}`,
        run_id: `work-generated-${index + 1}`,
        product_id: body.product_id,
        product_name: "Smart Light",
        title: variant.hook,
        hook: variant.hook,
        voice_profile_id: profile.id,
        voice_profile_name: profile.name,
        script_text: variant.script_text,
        duration_ms: 0,
        status: "generating",
        progress: 8,
        stage_label: "等待生成",
        created_at: "2026-07-15T09:10:00.000Z",
        editing_intent: variant.editing_intent,
        beats: variant.beats,
        bgm: variant.bgm.mode === "none" ? undefined : { track_id: variant.bgm.track_id || "bgm-light-1", name: variant.bgm.track_id === "bgm-warm-1" ? "温暖叙事" : "轻快骑行", gain_db: variant.bgm.gain_db }
      }));
      generatedWorkPolls = 0;
      finishedWorks = [...createdWorks, ...finishedWorks];
      await route.fulfill({
        status: 201,
        contentType: "application/json",
        body: JSON.stringify({ data: createdWorks })
      });
      return;
    }

    if (url.includes("/api/workbench/works/") && url.endsWith("/regenerate")) {
      const workID = url.split("/").at(-2);
      const regenerated = finishedWorks.find((work) => work.id === workID);
      if (regenerated) {
        Object.assign(regenerated, {
          status: "generating",
          progress: 8,
          stage_label: "生成旁白",
          completed_at: undefined,
          video_url: undefined
        });
      }
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({ data: regenerated })
      });
      return;
    }

    if (url.includes("/api/workbench/works/") && route.request().method() === "DELETE") {
      const workID = url.split("/").at(-1);
      finishedWorks = finishedWorks.filter((work) => work.id !== workID);
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({ data: { deleted: true } })
      });
      return;
    }

    if (url.includes("/api/workbench/works")) {
      if (finishedWorks.some((work) => work.id.startsWith("work-generated-"))) {
        generatedWorkPolls += 1;
        if (generatedWorkPolls >= 3) {
          finishedWorks = finishedWorks.map((work) => work.id.startsWith("work-generated-") ? {
            ...work,
            duration_ms: 6400,
            status: "completed",
            progress: 100,
            stage_label: "已完成",
            completed_at: "2026-07-15T09:10:08.000Z",
            audio_url: "/storage/voiceovers/work-generated-1.wav",
            narration_segments: [{ id: "generated-segment-1", start_ms: 0, end_ms: 6400, text: work.script_text }]
          } : work);
        }
      }
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({ data: finishedWorks })
      });
      return;
    }

    if (url.includes("/api/admin/model-providers")) {
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({ data: [] })
      });
      return;
    }

    if (url.includes("/api/admin/model-settings")) {
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            llm: { capability: "llm", provider_id: "", model: "" },
            vlm: { capability: "vlm", provider_id: "", model: "" },
            embedding: { capability: "embedding", provider_id: "", model: "", dimension: 1024 }
          }
        })
      });
      return;
    }

    if (url.includes("/api/admin/runtime-settings")) {
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            llm_max_concurrency: 1,
            vlm_max_concurrency: 1,
            asr_max_concurrency: 1,
            tts_max_concurrency: 1,
            render_max_concurrency: 1,
            task_max_queued_per_user: 1,
            task_max_running_per_user: 1,
            vlm_timeout_seconds: 30,
            vlm_max_retries: 0
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

  await expect(page.getByTestId("workbench-page")).toBeVisible();
  await expect(page.getByRole("menuitem", { name: "工作台" })).toBeVisible();
  await expect(page.getByRole("menuitem", { name: "成品库" })).toBeVisible();
  await expect(page.getByRole("menuitem", { name: "音乐库" })).toBeVisible();
  await expect(page.getByRole("menuitem", { name: "任务" })).toHaveCount(0);

  await page.getByRole("menuitem", { name: "音乐库" }).click();
  await expect(page.getByTestId("bgm-library-page")).toBeVisible();
  await expect(page.getByTestId("bgm-track-bgm-light-1")).toContainText("轻快骑行");
  await expect(page.getByTestId("bgm-track-bgm-warm-1")).toContainText("温暖叙事");
  await page.getByRole("menuitem", { name: "工作台" }).click();

  const voiceSelector = page.getByTestId("workbench-voice-selector");
  await expect(voiceSelector).toContainText("温和女声");
  await expect(page.getByTestId("workbench-subtitle-preset")).toContainText("信息流白字");
  await voiceSelector.getByRole("button", { name: "选择音色" }).click();
  const voiceModal = page.getByTestId("voice-profile-modal");
  await expect(voiceModal.getByTestId("voice-profile-option-voice-warm-female")).toBeVisible();
  await expect(voiceModal.getByTestId("voice-profile-option-voice-clear-male")).toBeVisible();
  await expect(voiceModal.getByTestId("voice-profile-option-voice-bright-female")).toBeVisible();
  await voiceModal.getByTestId("voice-profile-option-voice-clear-male").click();
  await expect(voiceSelector).toContainText("清晰男声");
  await page.reload();
  await expect(page.getByTestId("workbench-voice-selector")).toContainText("清晰男声");

  await page.getByRole("menuitem", { name: "设置" }).click();
  const voicesTab = page.getByRole("tab", { name: "旁白音色" });
  await expect(voicesTab).toBeVisible();
  await voicesTab.click();
  const voiceSettings = page.getByTestId("voice-profiles-settings");
  await expect(voiceSettings).toBeVisible();
  await expect(voiceSettings.getByTestId("voice-profile-settings-voice-warm-female")).toBeVisible();
  await expect(voiceSettings.getByTestId("voice-profile-settings-voice-clear-male")).toBeVisible();
  await expect(voiceSettings.getByTestId("voice-profile-settings-voice-bright-female")).toBeVisible();
  await page.getByRole("tab", { name: "成片样式" }).click();
  const subtitleStyleSettings = page.getByTestId("subtitle-style-settings");
  await expect(subtitleStyleSettings).toBeVisible();
  const subtitleStylePreview = page.getByLabel("信息流白字 9:16 字幕预览");
  await expect(subtitleStylePreview).toBeVisible();
  const verticalPosition = subtitleStyleSettings.getByRole("spinbutton", { name: "垂直位置数值" });
  const previewCaptionLayer = subtitleStylePreview.locator(".subtitle-style-preview-caption-layer");
  await verticalPosition.fill("20");
  await expect(previewCaptionLayer).toHaveAttribute("style", /top: 20%/);
  await verticalPosition.fill("80");
  await expect(previewCaptionLayer).toHaveAttribute("style", /top: 80%/);
  const verticalPositionSlider = subtitleStyleSettings.getByRole("slider", { name: "垂直位置滑块" });
  await verticalPositionSlider.focus();
  await verticalPositionSlider.press("Home");
  await expect(previewCaptionLayer).toHaveAttribute("style", /top: 5%/);
  await verticalPositionSlider.press("End");
  await expect(previewCaptionLayer).toHaveAttribute("style", /top: 95%/);
  const previewCaption = subtitleStylePreview.locator(".subtitle-style-preview-caption");
  const previewCaptionText = previewCaption.locator("span").first();
  await expect(previewCaption.locator("span")).toHaveCount(1);
  await expect(previewCaptionText).toHaveText("这款束裤带来帮你");
  await expect.poll(() => page.evaluate(() => document.fonts.check('700 16px "Noto Sans SC"'))).toBe(true);
  const backgroundSwitch = subtitleStyleSettings.getByRole("switch", { name: "背景" });
  const backgroundOpacity = subtitleStyleSettings.getByRole("spinbutton", { name: "背景不透明度" });
  await backgroundSwitch.click();
  await expect(backgroundOpacity).toBeDisabled();
  await expect(previewCaptionText).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
  const outlineSwitch = subtitleStyleSettings.getByRole("switch", { name: "描边" });
  const outlineWidth = subtitleStyleSettings.getByRole("spinbutton", { name: "描边宽度" });
  await expect(outlineWidth).toBeDisabled();
  await outlineSwitch.click();
  await expect(outlineWidth).toBeEnabled();
  await expect(previewCaption).not.toHaveCSS("-webkit-text-stroke-width", "0px");
  await outlineSwitch.click();
  await expect(previewCaption).toHaveCSS("-webkit-text-stroke-width", "0px");
  await subtitleStyleSettings.getByRole("button", { name: "保存" }).click();
  await expect(page.getByText("字幕样式已保存")).toBeVisible();
  expect(savedSubtitlePresetPayload?.layouts["9:16"].text_align).toBe("center");
  expect(savedSubtitlePresetPayload?.layouts["3:4"].text_align).toBe("center");
  expect(savedSubtitlePresetPayload?.background_opacity).toBe(0);
  expect(savedSubtitlePresetPayload?.outline_width).toBe(0);
  await page.getByRole("menuitem", { name: "用户管理" }).click();
  const usersPage = page.getByTestId("users-page");
  await expect(usersPage).toBeVisible();
  await expect(usersPage.getByText("Admin", { exact: true })).toBeVisible();
  await page.getByRole("menuitem", { name: "工作台" }).click();

  await page.getByRole("menuitem", { name: "成品库" }).click();
  const completedWork = page.getByTestId("finished-work-work-completed-1");
  await expect(completedWork).toBeVisible();
  const completedDetailButton = completedWork.getByRole("button");
  await expect(completedDetailButton).toHaveCount(1);
  await completedDetailButton.click();
  await expect(page.getByTestId("finished-work-detail")).toBeVisible();
  await expect(page.getByLabel("灯光自动唤醒，夜间更安心成品视频")).toBeVisible();
  await expect(page.getByRole("tab", { name: /概览/ })).toHaveAttribute("aria-selected", "true");
  await page.getByRole("tab", { name: /字幕/ }).click();
  await expect(page.getByText("回到家灯光自动亮起", { exact: true })).toBeVisible();
  await expect(page.getByText("无需摸黑找开关夜间使用更安心", { exact: true })).toBeVisible();
  await page.getByRole("tab", { name: /镜头编排/ }).click();
  await expect(page.getByText("自动唤醒", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "返回成品库" }).click();
  await expect(page.getByTestId("finished-library-page")).toBeVisible();

  await completedWork.click({ button: "right" });
  await expect(page.getByTestId("finished-library-page")).toBeVisible();
  await page.getByRole("menuitem", { name: "重新生成" }).click();
  const regenerateConfirm = page.locator(".ant-modal-confirm").filter({ hasText: "重新生成成片？" });
  await expect(regenerateConfirm).toBeVisible();
  await regenerateConfirm.getByRole("button", { name: "重新生成" }).click();
  await expect(completedWork).toHaveAttribute("data-status", "generating");

  const deleteWork = page.getByTestId("finished-work-work-delete-1");
  await deleteWork.click({ button: "right" });
  await page.getByRole("menuitem", { name: "删除成片" }).click();
  const deleteConfirm = page.locator(".ant-modal-confirm").filter({ hasText: "删除成片？" });
  await expect(deleteConfirm).toBeVisible();
  await deleteConfirm.locator(".ant-btn-dangerous").click();
  await expect(deleteWork).toHaveCount(0);

  await page.getByRole("menuitem", { name: "工作台" }).click();

  await page.getByTestId("workbench-product-select").click();
  await page.keyboard.press("ArrowDown");
  await page.keyboard.press("Enter");
  await expect(page.getByTestId("workbench-generate")).toBeEnabled();
  await page.getByTestId("workbench-generate").click();
  await expect(page.getByText("文案 01")).toBeVisible();
  const bgmControls = page.locator(".workbench-bgm");
  await expect(bgmControls.getByText("2 首可用")).toBeVisible();
  await bgmControls.getByText("指定", { exact: true }).click();
  await bgmControls.locator(".ant-select").click();
  await page.getByText(/温暖叙事 · 温暖 · 92 BPM/).click();
  await bgmControls.getByRole("spinbutton").fill("-10");
  await page.getByRole("button", { name: "试听当前文案" }).click();
  await expect(page.getByLabel("当前文案试听")).toBeVisible();
  await page.getByRole("button", { name: "确认文案" }).click();
  await expect(page.getByTestId("workbench-start-tasks")).toHaveText("开始 1 条任务");
  await page.getByTestId("workbench-start-tasks").click();
  expect((voiceoverTaskPayload?.variants as Array<any>)[0].bgm).toEqual({ mode: "track", track_id: "bgm-warm-1", gain_db: -10 });
  await expect(page.getByTestId("finished-library-page")).toBeVisible();
  await expect(page.getByTestId("finished-work-work-generated-1")).toHaveAttribute("data-status", "generating");
  await expect(page.getByText("待提交", { exact: true })).toHaveCount(0);
  await expect(page.getByText("已提交", { exact: true })).toHaveCount(0);

  await expect(page.locator('[data-status="completed"]')).toBeVisible();
  await expect(page.locator('[data-status="completed"] button')).toBeVisible();

  await page.getByRole("menuitem", { name: "素材" }).click();
  await expect(page.getByTestId("assets-page")).toBeVisible();
  await expect(page.getByText("clean-shot.mp4")).toBeVisible();
  await expect(page.getByText("mute-shot.mp4")).toBeVisible();

  await page.evaluate(() => {
    window.location.hash = "#/finished/work-generated-1";
  });
  await expect(page.getByTestId("finished-work-detail")).toBeVisible();
  await expect(page.locator(".finished-detail-header-meta").getByText("清晰男声", { exact: true })).toBeVisible();
  await expect(page.locator(".finished-detail-header-meta").getByText("温暖叙事 · -10 dB", { exact: true })).toBeVisible();
  await page.reload();
  await expect(page.getByTestId("finished-work-detail")).toBeVisible();
  await expect(page.locator(".finished-detail-header-meta").getByText("清晰男声", { exact: true })).toBeVisible();
});
