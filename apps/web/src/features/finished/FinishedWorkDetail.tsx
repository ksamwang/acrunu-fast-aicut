import { useEffect, useRef } from "react";
import type { WheelEvent as ReactWheelEvent } from "react";
import { Button, Progress, Tabs, Tag, Tooltip, Typography } from "antd";
import {
  ArrowLeft,
  Captions,
  CheckCircle2,
  ChevronDown,
  ChevronUp,
  CircleAlert,
  Clapperboard,
  Clock3,
  FileText,
  LoaderCircle,
  Mic2,
  MonitorPlay,
  Music2,
  RotateCcw,
  Trash2,
  UserRound,
  Volume2
} from "lucide-react";
import { formatDateTime, formatDuration, formatTimestamp } from "../../shared/lib/format";
import { subtitleDisplayText } from "../../shared/lib/subtitle";
import type { FinishedWork } from "../../shared/types/generation";
import { FinishedWorkVisual } from "./FinishedWorkVisual";

const sourceTypeLabels = {
  visual_only: "纯画面",
  talking_head: "口播",
  mixed: "混合"
};

type FinishedWorkDetailProps = {
  work: FinishedWork;
  position: number;
  total: number;
  actionKind: "retry" | "regenerate" | "delete" | null;
  onBack: () => void;
  onPrevious?: () => void;
  onNext?: () => void;
  onRetry: () => void;
  onRegenerate: () => void;
  onDelete: () => void;
};

function outputResolution(work: FinishedWork) {
  return work.output_width && work.output_height ? `${work.output_width} × ${work.output_height}` : "-";
}

function formatFileSize(bytes?: number) {
  if (!bytes || bytes <= 0) {
    return "-";
  }
  return `${(bytes / 1024 / 1024).toFixed(bytes >= 10 * 1024 * 1024 ? 0 : 1)} MB`;
}

export function FinishedWorkDetail({
  work,
  position,
  total,
  actionKind,
  onBack,
  onPrevious,
  onNext,
  onRetry,
  onRegenerate,
  onDelete
}: FinishedWorkDetailProps) {
  const isGenerating = work.status === "generating";
  const isFailed = work.status === "failed";
  const previewRef = useRef<HTMLDivElement | null>(null);
  const wheelDeltaRef = useRef(0);
  const wheelLockedUntilRef = useRef(0);
  const wheelResetTimerRef = useRef<number | null>(null);
  const playbackStateRef = useRef({ shouldResume: false, volume: 1, muted: false });
  const narrationSegments = work.narration_segments ?? [];
  const editPlan = work.edit_plan ?? [];
  const beats = work.beats ?? [];
  const clipTotalsByVisualBeat = editPlan.reduce<Map<string, number>>((totals, clip) => {
    const key = clip.visual_beat_id ?? clip.id;
    totals.set(key, (totals.get(key) ?? 0) + 1);
    return totals;
  }, new Map());
  const clipIndexesByVisualBeat = new Map<string, number>();
  const clipPositions = editPlan.map((clip) => {
    const key = clip.visual_beat_id ?? clip.id;
    const position = (clipIndexesByVisualBeat.get(key) ?? 0) + 1;
    clipIndexesByVisualBeat.set(key, position);
    return { position, total: clipTotalsByVisualBeat.get(key) ?? 1 };
  });
  const previewCaption = narrationSegments[0] ? subtitleDisplayText(narrationSegments[0].text) : "";
  const statusLabel = isGenerating ? "生成中" : isFailed ? "生成失败" : "已完成";
  const actionBusy = actionKind !== null;

  useEffect(() => {
    wheelDeltaRef.current = 0;
    const video = previewRef.current?.querySelector("video");
    if (!video) {
      return;
    }
    const playback = playbackStateRef.current;
    video.volume = playback.volume;
    video.muted = playback.muted;
    if (!playback.shouldResume) {
      return;
    }
    const resume = () => {
      void video.play().catch(() => {
        playbackStateRef.current.shouldResume = false;
      });
    };
    if (video.readyState >= HTMLMediaElement.HAVE_CURRENT_DATA) {
      resume();
      return;
    }
    video.addEventListener("canplay", resume, { once: true });
    return () => video.removeEventListener("canplay", resume);
  }, [work.id, work.video_url]);

  useEffect(() => () => {
    if (wheelResetTimerRef.current !== null) {
      window.clearTimeout(wheelResetTimerRef.current);
    }
  }, []);

  const navigateToWork = (navigate?: () => void) => {
    if (!navigate) {
      return;
    }
    const video = previewRef.current?.querySelector("video");
    if (video) {
      playbackStateRef.current = {
        shouldResume: !video.paused && !video.ended,
        volume: video.volume,
        muted: video.muted
      };
    }
    navigate();
  };

  const handlePreviewWheel = (event: ReactWheelEvent<HTMLDivElement>) => {
    if (event.deltaY === 0 || Math.abs(event.deltaY) < Math.abs(event.deltaX)) {
      return;
    }
    event.preventDefault();
    const now = window.performance.now();
    if (now < wheelLockedUntilRef.current) {
      return;
    }
    const unit = event.deltaMode === 1 ? 16 : event.deltaMode === 2 ? event.currentTarget.clientHeight : 1;
    const delta = event.deltaY * unit;
    if (wheelDeltaRef.current !== 0 && Math.sign(wheelDeltaRef.current) !== Math.sign(delta)) {
      wheelDeltaRef.current = 0;
    }
    wheelDeltaRef.current += delta;
    if (wheelResetTimerRef.current !== null) {
      window.clearTimeout(wheelResetTimerRef.current);
    }
    wheelResetTimerRef.current = window.setTimeout(() => {
      wheelDeltaRef.current = 0;
      wheelResetTimerRef.current = null;
    }, 180);
    if (Math.abs(wheelDeltaRef.current) < 80) {
      return;
    }
    const navigate = wheelDeltaRef.current > 0 ? onNext : onPrevious;
    wheelDeltaRef.current = 0;
    if (!navigate) {
      return;
    }
    wheelLockedUntilRef.current = now + 420;
    navigateToWork(navigate);
  };

  const overviewPane = (
    <div className="finished-detail-pane-scroll">
      <section className="finished-detail-copy-section">
        <div className="finished-detail-copy-heading">
          <FileText size={14} />
          <Typography.Text>旁白文案</Typography.Text>
        </div>
        <Typography.Paragraph className="finished-detail-script">{work.script_text}</Typography.Paragraph>
      </section>

      {work.editing_intent ? (
        <section className="finished-detail-copy-section">
          <div className="finished-detail-copy-heading">
            <Clapperboard size={14} />
            <Typography.Text>编排意图</Typography.Text>
          </div>
          <Typography.Paragraph className="finished-detail-intent">{work.editing_intent}</Typography.Paragraph>
        </section>
      ) : null}

      <dl className="finished-detail-facts">
        <div>
          <dt>产品</dt>
          <dd>{work.product_name}</dd>
        </div>
        <div>
          <dt>创建人</dt>
          <dd>{work.created_by_name || "未知用户"}</dd>
        </div>
        <div>
          <dt>音色</dt>
          <dd>{work.voice_profile_name || "-"}</dd>
        </div>
        <div>
          <dt>背景音乐</dt>
          <dd>{work.bgm ? `${work.bgm.name} · ${work.bgm.gain_db} dB` : "无"}</dd>
        </div>
        <div>
          <dt>时长</dt>
          <dd>{formatDuration(work.duration_ms)}</dd>
        </div>
        <div>
          <dt>输出</dt>
          <dd>{outputResolution(work)}</dd>
        </div>
        <div>
          <dt>文件</dt>
          <dd>{formatFileSize(work.output_file_size_bytes)}</dd>
        </div>
        <div>
          <dt>{work.completed_at ? "完成时间" : "创建时间"}</dt>
          <dd>{formatDateTime(work.completed_at ?? work.created_at)}</dd>
        </div>
      </dl>

      {isGenerating ? (
        <div className="finished-detail-progress">
          <span>{work.stage_label}</span>
          <span>{work.progress}%</span>
          <Progress percent={work.progress} showInfo={false} size="small" strokeColor="#2f8c83" />
        </div>
      ) : isFailed ? (
        <div className="finished-detail-failure">
          <CircleAlert size={16} />
          <span>{work.error_message || "生成失败"}</span>
        </div>
      ) : (
        <div className="finished-detail-complete">
          <CheckCircle2 size={16} />
          <span>成品已完成</span>
        </div>
      )}
    </div>
  );

  const captionPane = (
    <div className="finished-detail-pane-scroll">
      <div className="finished-detail-pane-summary">
        <span>{narrationSegments.length} 个时间段</span>
        <span>{formatDuration(work.duration_ms)}</span>
      </div>
      <div className="finished-detail-caption-list">
        {narrationSegments.map((segment, index) => (
          <article className="finished-detail-caption-row" key={segment.id}>
            <span className="finished-detail-row-index">{String(index + 1).padStart(2, "0")}</span>
            <span className="finished-detail-caption-time">
              {formatTimestamp(segment.start_ms)} - {formatTimestamp(segment.end_ms)}
            </span>
            <span className="finished-detail-caption-copy">{subtitleDisplayText(segment.text)}</span>
          </article>
        ))}
      </div>
    </div>
  );

  const planPane = (
    <div className="finished-detail-pane-scroll">
      <div className="finished-detail-pane-summary">
        <span>{editPlan.length || beats.length} 个镜头</span>
        <span>{formatDuration(work.duration_ms)}</span>
      </div>
      <div className="finished-detail-plan-list">
        {editPlan.map((clip, index) => (
          <article className="finished-detail-plan-row" key={clip.id}>
            <span className="finished-detail-row-index">{String(index + 1).padStart(2, "0")}</span>
            <span className="finished-detail-plan-time">
              {formatTimestamp(clip.start_ms)} - {formatTimestamp(clip.end_ms)}
              {clip.source_in_ms !== undefined && clip.source_out_ms !== undefined ? (
                <small>源 {formatTimestamp(clip.source_in_ms)} - {formatTimestamp(clip.source_out_ms)}</small>
              ) : null}
            </span>
            <span className="finished-detail-plan-content">
              <strong>{clip.label}</strong>
              <span>{clip.visual_goal}</span>
            </span>
            <span className="finished-detail-plan-tags">
              {clipPositions[index].total > 1 ? <Tag color="cyan">同段 {clipPositions[index].position}/{clipPositions[index].total}</Tag> : null}
              <Tag>{sourceTypeLabels[clip.source_type]}</Tag>
              {clip.use_original_audio ? <Tag color="blue">原声</Tag> : null}
            </span>
          </article>
        ))}
        {editPlan.length === 0 ? beats.map((beat, index) => (
          <article className="finished-detail-plan-row is-intent" key={beat.id}>
            <span className="finished-detail-row-index">{String(index + 1).padStart(2, "0")}</span>
            <span className="finished-detail-plan-time">{beat.selling_point}</span>
            <span className="finished-detail-plan-content">
              <strong>{beat.label}</strong>
              <span>{beat.visual_goal}</span>
            </span>
            <span className="finished-detail-plan-tags">
              <Tag>{sourceTypeLabels[beat.source_type]}</Tag>
            </span>
          </article>
        )) : null}
      </div>
    </div>
  );

  return (
    <div className="finished-detail-page" data-testid="finished-work-detail">
      <header className="finished-detail-header">
        <Tooltip title="返回成品库">
          <Button type="text" aria-label="返回成品库" icon={<ArrowLeft size={18} />} onClick={onBack} />
        </Tooltip>
        <div className="finished-detail-heading">
          <div className="finished-detail-title-row">
            <Typography.Text className="finished-detail-title">{work.title}</Typography.Text>
            <Tag className="finished-detail-product">{work.product_name}</Tag>
            <Tag color={isGenerating ? "processing" : isFailed ? "error" : "green"}>{statusLabel}</Tag>
          </div>
          <div className="finished-detail-header-meta">
            <span><Clock3 size={13} />{formatDuration(work.duration_ms)}</span>
            {work.voice_profile_name ? <span><Mic2 size={13} />{work.voice_profile_name}</span> : null}
            <span><UserRound size={13} />{work.created_by_name || "未知用户"}</span>
            {work.bgm ? <span><Music2 size={13} />{work.bgm.name} · {work.bgm.gain_db} dB</span> : null}
            {work.output_width && work.output_height ? <span><MonitorPlay size={13} />{outputResolution(work)}</span> : null}
            <span>{formatDateTime(work.completed_at ?? work.created_at)}</span>
          </div>
        </div>
        <div className="finished-detail-actions">
          <Button
            icon={<RotateCcw size={15} />}
            loading={actionKind === "retry" || actionKind === "regenerate"}
            disabled={isGenerating || actionBusy}
            onClick={isFailed ? onRetry : onRegenerate}
          >
            {isFailed ? "重试" : "重新生成"}
          </Button>
          <Button
            danger
            icon={<Trash2 size={15} />}
            loading={actionKind === "delete"}
            disabled={isGenerating || actionBusy}
            onClick={onDelete}
          >
            删除
          </Button>
        </div>
      </header>

      <main className="finished-detail-scroll">
        <div className="finished-detail-workspace">
          <section className="finished-detail-media-panel" aria-label="成品预览">
            <div
              className={`finished-detail-preview${isGenerating ? " is-generating" : ""}`}
              ref={previewRef}
              onWheel={handlePreviewWheel}
            >
              <FinishedWorkVisual key={work.id} work={work} />
              {!work.video_url ? <span className="finished-detail-preview-scrim" /> : null}
              {isGenerating ? (
                <span className="finished-detail-generation-state">
                  <LoaderCircle size={22} />
                  <span>{work.stage_label}</span>
                </span>
              ) : isFailed ? (
                <span className="finished-detail-generation-state is-failed">
                  <CircleAlert size={22} />
                  <span>{work.error_message || "生成失败"}</span>
                </span>
              ) : work.video_url ? null : (
                <span className="finished-detail-play"><Volume2 size={22} /></span>
              )}
              {previewCaption && !work.video_url ? <span className="finished-detail-caption">{previewCaption}</span> : null}
              <nav className="finished-detail-preview-navigation" aria-label="切换成品">
                <Tooltip title="上一条成品" placement="left">
                  <Button
                    type="text"
                    icon={<ChevronUp size={18} />}
                    aria-label="上一条成品"
                    disabled={!onPrevious}
                    onClick={() => navigateToWork(onPrevious)}
                  />
                </Tooltip>
                <span className="finished-detail-preview-position" aria-label={`第 ${position} 条，共 ${total} 条`}>
                  {position}<small>/</small>{total}
                </span>
                <Tooltip title="下一条成品" placement="left">
                  <Button
                    type="text"
                    icon={<ChevronDown size={18} />}
                    aria-label="下一条成品"
                    disabled={!onNext}
                    onClick={() => navigateToWork(onNext)}
                  />
                </Tooltip>
              </nav>
            </div>

            <div className="finished-detail-media-footer">
              <span className="finished-detail-media-stage">
                {isGenerating ? <LoaderCircle size={14} /> : isFailed ? <CircleAlert size={14} /> : <CheckCircle2 size={14} />}
                {work.stage_label}
              </span>
              <span>{formatDuration(work.duration_ms)}</span>
            </div>
            {isGenerating ? <Progress percent={work.progress} showInfo={false} size="small" strokeColor="#4fc1b2" /> : null}
            {work.audio_url && !work.video_url ? <audio className="finished-detail-audio" controls preload="metadata" src={work.audio_url} aria-label="旁白音频" /> : null}
          </section>

          <aside className="finished-detail-inspector" aria-label="成品信息">
            <Tabs
              size="small"
              defaultActiveKey="overview"
              items={[
                { key: "overview", label: <span className="finished-detail-tab-label"><FileText size={14} />概览</span>, children: overviewPane },
                { key: "captions", label: <span className="finished-detail-tab-label"><Captions size={14} />字幕 <b>{narrationSegments.length}</b></span>, children: captionPane },
                { key: "plan", label: <span className="finished-detail-tab-label"><Clapperboard size={14} />镜头编排 <b>{editPlan.length || beats.length}</b></span>, children: planPane }
              ]}
            />
          </aside>
        </div>
      </main>
    </div>
  );
}
