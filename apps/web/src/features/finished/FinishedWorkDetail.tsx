import { Button, Progress, Tag, Tooltip, Typography } from "antd";
import { ArrowLeft, CheckCircle2, CircleAlert, LoaderCircle, Volume2 } from "lucide-react";
import { formatDateTime, formatDuration, formatTimestamp } from "../../shared/lib/format";
import type { FinishedWork } from "../../shared/types/generation";
import { FinishedWorkVisual } from "./FinishedWorkVisual";

const sourceTypeLabels = {
  visual_only: "纯画面",
  talking_head: "口播",
  mixed: "混合"
};

export function FinishedWorkDetail({ work, onBack }: { work: FinishedWork; onBack: () => void }) {
  const isGenerating = work.status === "generating";
  const isFailed = work.status === "failed";
  const narrationSegments = work.narration_segments ?? [];
  const editPlan = work.edit_plan ?? [];
  const beats = work.beats ?? [];
  const previewCaption = narrationSegments[0]?.text;

  return (
    <div className="finished-detail-page" data-testid="finished-work-detail">
      <header className="finished-detail-header">
        <Tooltip title="返回成品库">
          <Button type="text" aria-label="返回成品库" icon={<ArrowLeft size={19} />} onClick={onBack} />
        </Tooltip>
        <div className="finished-detail-heading">
          <Typography.Text className="finished-detail-title">{work.title}</Typography.Text>
          <div className="finished-detail-heading-meta">
            <Tag className="finished-detail-product">{work.product_name}</Tag>
            <Tag color={isGenerating ? "processing" : isFailed ? "error" : "green"}>{isGenerating ? "生成中" : isFailed ? "生成失败" : "已完成"}</Tag>
          </div>
        </div>
      </header>

      <main className="finished-detail-scroll">
        <section className="finished-detail-overview">
          <div className="finished-detail-preview-column">
            <div className={`finished-detail-preview${isGenerating ? " is-generating" : ""}`}>
              <FinishedWorkVisual work={work} />
              <span className="finished-detail-preview-scrim" />
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
              ) : (
                <span className="finished-detail-play"><Volume2 size={23} /></span>
              )}
              {previewCaption ? <span className="finished-detail-caption">{previewCaption}</span> : null}
            </div>
            <div className="finished-detail-timebar" aria-label="成品时间轴">
              <span>0:00</span>
              <span className="finished-detail-timebar-track">
                <span className="finished-detail-timebar-progress" style={{ width: `${isGenerating ? work.progress : 100}%` }} />
              </span>
              <span>{formatDuration(work.duration_ms)}</span>
            </div>
            {work.audio_url ? <audio className="finished-detail-audio" controls preload="metadata" src={work.audio_url} aria-label="旁白音频" /> : null}
          </div>

          <aside className="finished-detail-summary">
            <section className="finished-detail-summary-section">
              <Typography.Text className="finished-detail-section-label">旁白</Typography.Text>
              <Typography.Paragraph className="finished-detail-script">{work.script_text}</Typography.Paragraph>
            </section>
            <section className="finished-detail-summary-section">
              <Typography.Text className="finished-detail-section-label">编排意图</Typography.Text>
              <Typography.Paragraph className="finished-detail-intent">{work.editing_intent}</Typography.Paragraph>
            </section>
            <dl className="finished-detail-facts">
              <div>
                <dt>时长</dt>
                <dd>{formatDuration(work.duration_ms)}</dd>
              </div>
              <div>
                <dt>旁白</dt>
                <dd>{narrationSegments.length} 句</dd>
              </div>
              {work.voice_profile_name ? (
                <div>
                  <dt>音色</dt>
                  <dd>{work.voice_profile_name}</dd>
                </div>
              ) : null}
              <div>
                <dt>完成时间</dt>
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
              <span className="finished-detail-failed"><CircleAlert size={16} /> {work.error_message || "生成失败"}</span>
            ) : (
              <span className="finished-detail-complete"><CheckCircle2 size={16} /> 旁白已完成</span>
            )}
          </aside>
        </section>

        <section className="finished-detail-section" aria-label="字幕">
          <div className="finished-detail-section-heading">
            <Typography.Text>字幕</Typography.Text>
            <Typography.Text type="secondary">{narrationSegments.length} 句</Typography.Text>
          </div>
          <div className="finished-detail-caption-list">
            {narrationSegments.map((segment) => (
              <article className="finished-detail-caption-row" key={segment.id}>
                <span className="finished-detail-caption-time">{formatTimestamp(segment.start_ms)}</span>
                <span className="finished-detail-caption-copy">{segment.text}</span>
                <span className="finished-detail-caption-end">{formatTimestamp(segment.end_ms)}</span>
              </article>
            ))}
          </div>
        </section>

        {editPlan.length > 0 || beats.length > 0 ? (
          <section className="finished-detail-section" aria-label="初步镜头编排">
            <div className="finished-detail-section-heading">
              <Typography.Text>初步镜头编排</Typography.Text>
              <Typography.Text type="secondary">{editPlan.length || beats.length} 段</Typography.Text>
            </div>
            <div className="finished-detail-plan-list">
              {editPlan.map((beat) => (
                <article className="finished-detail-plan-row" key={beat.id}>
                  <span className="finished-detail-plan-time">{formatTimestamp(beat.start_ms)} - {formatTimestamp(beat.end_ms)}</span>
                  <span className="finished-detail-plan-label">{beat.label}</span>
                  <span className="finished-detail-plan-goal">{beat.visual_goal}</span>
                  <Tag>{sourceTypeLabels[beat.source_type]}</Tag>
                </article>
              ))}
              {editPlan.length === 0 ? beats.map((beat) => (
                <article className="finished-detail-plan-row is-intent" key={beat.id}>
                  <span className="finished-detail-plan-time">{beat.selling_point}</span>
                  <span className="finished-detail-plan-label">{beat.label}</span>
                  <span className="finished-detail-plan-goal">{beat.visual_goal}</span>
                  <Tag>{sourceTypeLabels[beat.source_type]}</Tag>
                </article>
              )) : null}
            </div>
          </section>
        ) : null}
      </main>
    </div>
  );
}
