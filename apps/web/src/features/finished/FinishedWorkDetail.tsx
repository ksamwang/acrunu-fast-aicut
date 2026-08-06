import { useEffect, useRef, useState } from "react";
import type { WheelEvent as ReactWheelEvent } from "react";
import { Alert, Button, Empty, Input, message, Modal, Progress, Segmented, Slider, Tabs, Tag, Tooltip, Typography } from "antd";
import {
  ArrowLeft,
  Captions,
  Check,
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
  RefreshCw,
  RotateCcw,
  Search,
  Trash2,
  Undo2,
  UserRound,
  Volume2,
  X
} from "lucide-react";
import { formatDateTime, formatDuration, formatTimestamp } from "../../shared/lib/format";
import { subtitleDisplayText } from "../../shared/lib/subtitle";
import type { EditPlanBeat, FinishedWork, FinishedWorkClipCandidate, FinishedWorkClipCandidates, FinishedWorkClipReplacement, VoiceoverReplacement } from "../../shared/types/generation";
import { applyVoiceoverReplacement, cancelVoiceoverReplacement, createVoiceoverReplacement, getCurrentVoiceoverReplacement, getFinishedWork, listFinishedWorkClipCandidates, replaceFinishedWorkClips } from "./api";
import { FinishedWorkVisual } from "./FinishedWorkVisual";

const sourceTypeLabels = {
  visual_only: "纯画面",
  talking_head: "口播",
  mixed: "混合"
};

type FinishedWorkDetailProps = {
  work: FinishedWork;
  token: string;
  position: number;
  total: number;
  actionKind: "retry" | "regenerate" | "delete" | null;
  onBack: () => void;
  onPrevious?: () => void;
  onNext?: () => void;
  onRetry: () => void;
  onRegenerate: () => void;
  onDelete: () => void;
  onWorkUpdated: (work: FinishedWork) => void;
};

type PendingClipReplacement = FinishedWorkClipReplacement & {
  candidate: FinishedWorkClipCandidate;
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
  token,
  position,
  total,
  actionKind,
  onBack,
  onPrevious,
  onNext,
  onRetry,
  onRegenerate,
  onDelete,
  onWorkUpdated
}: FinishedWorkDetailProps) {
  const isGenerating = work.status === "generating";
  const isFailed = work.status === "failed";
  const previewRef = useRef<HTMLDivElement | null>(null);
  const wheelDeltaRef = useRef(0);
  const wheelLockedUntilRef = useRef(0);
  const wheelResetTimerRef = useRef<number | null>(null);
  const playbackStateRef = useRef({ shouldResume: false, volume: 1, muted: false });
  const candidateRequestRef = useRef(0);
  const [activeTab, setActiveTab] = useState("overview");
  const [selectedClipID, setSelectedClipID] = useState("");
  const [replacementClipID, setReplacementClipID] = useState("");
  const [candidateQuery, setCandidateQuery] = useState("");
  const [candidateData, setCandidateData] = useState<FinishedWorkClipCandidates | null>(null);
  const [candidateLoading, setCandidateLoading] = useState(false);
  const [candidateError, setCandidateError] = useState("");
  const [previewMode, setPreviewMode] = useState<"work" | "candidate">("work");
  const [previewCandidate, setPreviewCandidate] = useState<FinishedWorkClipCandidate | null>(null);
  const [candidateSourceInMs, setCandidateSourceInMs] = useState(0);
  const [pendingReplacements, setPendingReplacements] = useState<Record<string, PendingClipReplacement>>({});
  const [pendingPlanUpdatedAt, setPendingPlanUpdatedAt] = useState("");
  const [applyingReplacements, setApplyingReplacements] = useState(false);
  const [voiceoverModalOpen, setVoiceoverModalOpen] = useState(false);
  const [voiceoverReplacement, setVoiceoverReplacement] = useState<VoiceoverReplacement | null>(null);
  const [voiceoverAction, setVoiceoverAction] = useState<"generate" | "apply" | "cancel" | null>(null);
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
  const statusLabel = isGenerating ? (work.video_url ? "更新中" : "生成中") : isFailed ? "生成失败" : "已完成";
  const actionBusy = actionKind !== null;
  const selectedClip = editPlan.find((clip) => clip.id === selectedClipID);
  const replacementClip = editPlan.find((clip) => clip.id === replacementClipID);
  const pendingReplacementCount = Object.keys(pendingReplacements).length;
  const replacementPending = replacementClipID ? pendingReplacements[replacementClipID] : undefined;
  const pendingAssetIDs = new Set(
    Object.values(pendingReplacements)
      .filter((replacement) => replacement.clip_id !== replacementClipID)
      .map((replacement) => replacement.asset_id)
  );
  const displayedCandidates = candidateData?.items ?? [];
  const previewPendingElsewhere = Boolean(previewCandidate && pendingAssetIDs.has(previewCandidate.asset_id));
  const previewUnavailableReason = previewPendingElsewhere
    ? "已在其它待应用镜头中使用"
    : previewCandidate?.unavailable_reason ?? "";
  const previewSelectable = Boolean(previewCandidate?.selectable && !previewPendingElsewhere);
  const previewSourceDurationMs = candidateData && previewCandidate
    ? Math.min(candidateData.clip_duration_ms, Math.max(0, previewCandidate.duration_ms - candidateSourceInMs))
    : 0;
  const previewShortfallMs = candidateData
    ? Math.max(0, candidateData.clip_duration_ms - previewSourceDurationMs)
    : 0;
  const previewMatchesCurrent = Boolean(
    candidateData && previewCandidate && previewCandidate.asset_id === candidateData.current.asset_id &&
    candidateSourceInMs === candidateData.current.source_in_ms
  );
  const previewMatchesPending = Boolean(
    replacementPending && previewCandidate && replacementPending.asset_id === previewCandidate.asset_id &&
    replacementPending.source_in_ms === candidateSourceInMs
  );

  useEffect(() => {
    if (pendingReplacementCount === 0 && pendingPlanUpdatedAt) {
      setPendingPlanUpdatedAt("");
    }
  }, [pendingPlanUpdatedAt, pendingReplacementCount]);

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

  useEffect(() => {
    candidateRequestRef.current += 1;
    setActiveTab("overview");
    setSelectedClipID("");
    setReplacementClipID("");
    setCandidateQuery("");
    setCandidateData(null);
    setCandidateError("");
    setPreviewMode("work");
    setPreviewCandidate(null);
    setCandidateSourceInMs(0);
    setPendingReplacements({});
    setPendingPlanUpdatedAt("");
    setApplyingReplacements(false);
    setVoiceoverModalOpen(false);
    setVoiceoverReplacement(null);
    setVoiceoverAction(null);
  }, [work.id]);

  useEffect(() => {
    if (!voiceoverModalOpen || !voiceoverReplacement || (voiceoverReplacement.status !== "generating" && voiceoverReplacement.status !== "applying")) {
      return;
    }
    let cancelled = false;
    const timer = window.setInterval(() => {
      void getCurrentVoiceoverReplacement(work.id, token).then(async (current) => {
        if (cancelled || !current || current.id !== voiceoverReplacement.id) {
          return;
        }
        setVoiceoverReplacement(current);
        if (current.status === "applied") {
          const updated = await getFinishedWork(work.id, token);
          if (!cancelled) {
            onWorkUpdated(updated);
            setVoiceoverModalOpen(false);
            setVoiceoverReplacement(null);
            setVoiceoverAction(null);
            message.success("新配音已应用");
          }
        }
      }).catch((error) => {
        if (!cancelled) {
          message.error(error instanceof Error ? error.message : "读取配音状态失败");
        }
      });
    }, 1500);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [onWorkUpdated, token, voiceoverModalOpen, voiceoverReplacement, work.id]);

  const generateVoiceoverReplacement = async () => {
    setVoiceoverModalOpen(true);
    setVoiceoverAction("generate");
    try {
      const replacement = await createVoiceoverReplacement(work.id, token);
      setVoiceoverReplacement(replacement);
    } catch (error) {
      setVoiceoverModalOpen(false);
      setVoiceoverReplacement(null);
      message.error(error instanceof Error ? error.message : "重新生成配音失败");
    } finally {
      setVoiceoverAction(null);
    }
  };

  const applyGeneratedVoiceover = async () => {
    if (!voiceoverReplacement || voiceoverReplacement.status !== "ready") {
      return;
    }
    setVoiceoverAction("apply");
    try {
      const applying = await applyVoiceoverReplacement(work.id, voiceoverReplacement.id, token);
      setVoiceoverReplacement(applying);
    } catch (error) {
      const current = await getCurrentVoiceoverReplacement(work.id, token).catch(() => null);
      if (current) {
        setVoiceoverReplacement(current);
      }
      message.error(error instanceof Error ? error.message : "应用新配音失败");
    } finally {
      setVoiceoverAction(null);
    }
  };

  const closeVoiceoverReplacement = async () => {
    if (voiceoverReplacement?.status === "applying") {
      return;
    }
    setVoiceoverAction("cancel");
    try {
      if (voiceoverReplacement && voiceoverReplacement.status !== "applied" && voiceoverReplacement.status !== "cancelled") {
        await cancelVoiceoverReplacement(work.id, voiceoverReplacement.id, token);
      }
      setVoiceoverModalOpen(false);
      setVoiceoverReplacement(null);
    } catch (error) {
      message.error(error instanceof Error ? error.message : "取消配音草稿失败");
    } finally {
      setVoiceoverAction(null);
    }
  };

  useEffect(() => {
    const video = previewRef.current?.querySelector("video");
    if (!video) {
      return;
    }
    const clip = previewMode === "work" ? selectedClip : replacementClip;
    if (!clip) {
      return;
    }
    const startMs = previewMode === "candidate" ? candidateSourceInMs : clip.start_ms;
    const endMs = previewMode === "candidate"
      ? candidateSourceInMs + previewSourceDurationMs
      : clip.end_ms;
    const seekToStart = () => {
      video.currentTime = startMs / 1000;
      void video.play().catch(() => undefined);
    };
    const handleTimeUpdate = () => {
      if (video.currentTime * 1000 >= endMs - 30) {
        seekToStart();
      }
    };
    if (video.readyState >= HTMLMediaElement.HAVE_METADATA) {
      seekToStart();
    } else {
      video.addEventListener("loadedmetadata", seekToStart, { once: true });
    }
    video.addEventListener("timeupdate", handleTimeUpdate);
    return () => {
      video.removeEventListener("loadedmetadata", seekToStart);
      video.removeEventListener("timeupdate", handleTimeUpdate);
    };
  }, [candidateSourceInMs, previewCandidate?.asset_id, previewMode, previewSourceDurationMs, replacementClip, selectedClip, work.video_url]);

  const runAfterPendingDecision = (action: () => void) => {
    if (pendingReplacementCount === 0) {
      action();
      return;
    }
    Modal.confirm({
      title: "放弃未应用的镜头修改？",
      content: `还有 ${pendingReplacementCount} 个镜头修改尚未应用。`,
      okText: "放弃并离开",
      cancelText: "继续编辑",
      okButtonProps: { danger: true },
      centered: true,
      onOk: () => {
        setPendingReplacements({});
        setPendingPlanUpdatedAt("");
        action();
      }
    });
  };

  const navigateToWork = (navigate?: () => void) => {
    if (!navigate) {
      return;
    }
    runAfterPendingDecision(() => {
      const video = previewRef.current?.querySelector("video");
      if (video) {
        playbackStateRef.current = {
          shouldResume: !video.paused && !video.ended,
          volume: video.volume,
          muted: video.muted
        };
      }
      navigate();
    });
  };

  const handlePreviewWheel = (event: ReactWheelEvent<HTMLDivElement>) => {
    if (previewMode === "candidate") {
      return;
    }
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

  const selectClip = (clip: EditPlanBeat) => {
    setSelectedClipID(clip.id);
    setPreviewMode("work");
  };

  const loadClipCandidates = async (clip: EditPlanBeat, query: string) => {
    const requestID = candidateRequestRef.current + 1;
    candidateRequestRef.current = requestID;
    setCandidateLoading(true);
    setCandidateError("");
    try {
      const response = await listFinishedWorkClipCandidates(work.id, clip.id, query, token);
      if (candidateRequestRef.current !== requestID) {
        return;
      }
      setCandidateData(response);
      setCandidateQuery(response.query);
      const pending = pendingReplacements[clip.id];
      if (pending) {
        setPreviewCandidate(pending.candidate);
        setCandidateSourceInMs(pending.source_in_ms);
      } else {
        setPreviewCandidate(response.current);
        setCandidateSourceInMs(response.current.source_in_ms);
      }
    } catch (error) {
      if (candidateRequestRef.current === requestID) {
        setCandidateData(null);
        setCandidateError(error instanceof Error ? error.message : "候选素材加载失败");
      }
    } finally {
      if (candidateRequestRef.current === requestID) {
        setCandidateLoading(false);
      }
    }
  };

  const openClipReplacement = (clip: EditPlanBeat) => {
    selectClip(clip);
    setActiveTab("plan");
    setReplacementClipID(clip.id);
    setCandidateQuery(clip.visual_goal);
    setCandidateData(null);
    setPreviewMode("work");
    void loadClipCandidates(clip, "");
  };

  const closeClipReplacement = () => {
    candidateRequestRef.current += 1;
    setReplacementClipID("");
    setCandidateData(null);
    setCandidateError("");
    setPreviewCandidate(null);
    setPreviewMode("work");
  };

  const selectCandidate = (candidate: FinishedWorkClipCandidate) => {
    setPreviewCandidate(candidate);
    const pending = replacementClipID ? pendingReplacements[replacementClipID] : undefined;
    setCandidateSourceInMs(pending?.asset_id === candidate.asset_id ? pending.source_in_ms : candidate.source_in_ms);
    setPreviewMode("candidate");
  };

  const stageCandidate = () => {
    if (!replacementClip || !candidateData || !previewCandidate) {
      return;
    }
    if (!previewSelectable) {
      message.warning(previewUnavailableReason || "当前素材不能用于这个镜头");
      return;
    }
    const sourceInMs = Math.max(0, Math.min(previewCandidate.max_source_in_ms, Math.round(candidateSourceInMs)));
    if (pendingPlanUpdatedAt && pendingPlanUpdatedAt !== candidateData.plan_updated_at) {
      message.warning("镜头编排已发生变化，请放弃当前修改后重新选择");
      return;
    }
    if (!pendingPlanUpdatedAt) {
      setPendingPlanUpdatedAt(candidateData.plan_updated_at);
    }
    setPendingReplacements((current) => {
      const next = { ...current };
      if (previewCandidate.asset_id === candidateData.current.asset_id && sourceInMs === candidateData.current.source_in_ms) {
        delete next[replacementClip.id];
      } else {
        next[replacementClip.id] = {
          clip_id: replacementClip.id,
          asset_id: previewCandidate.asset_id,
          source_in_ms: sourceInMs,
          candidate: previewCandidate
        };
      }
      return next;
    });
  };

  const restoreClipMaterial = (clipID: string) => {
    if (pendingReplacements[clipID] && pendingReplacementCount === 1) {
      setPendingPlanUpdatedAt("");
    }
    setPendingReplacements((current) => {
      const next = { ...current };
      delete next[clipID];
      return next;
    });
    if (replacementClipID === clipID && candidateData) {
      setPreviewCandidate(candidateData.current);
      setCandidateSourceInMs(candidateData.current.source_in_ms);
      setPreviewMode("work");
    }
  };

  const applyClipReplacements = async () => {
    const planUpdatedAt = pendingPlanUpdatedAt;
    const replacements = Object.values(pendingReplacements).map(({ clip_id, asset_id, source_in_ms }) => ({
      clip_id, asset_id, source_in_ms
    }));
    if (!planUpdatedAt || replacements.length === 0) {
      return;
    }
    setApplyingReplacements(true);
    try {
      const updated = await replaceFinishedWorkClips(work.id, planUpdatedAt, replacements, token);
      onWorkUpdated(updated);
      setPendingReplacements({});
      setPendingPlanUpdatedAt("");
      closeClipReplacement();
      message.success(`已提交 ${replacements.length} 个镜头修改，正在重新渲染`);
    } catch (error) {
      message.error(error instanceof Error ? error.message : "镜头替换提交失败");
    } finally {
      setApplyingReplacements(false);
    }
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
      ) : work.error_message ? (
        <div className="finished-detail-revision-warning">
          <CircleAlert size={16} />
          <span>上次镜头更新失败，当前仍为原成片：{work.error_message}</span>
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
    <div className={`finished-detail-plan-editor${replacementClip ? " is-replacing" : ""}${pendingReplacementCount > 0 ? " has-pending" : ""}`}>
      <section className="finished-detail-plan-column">
        <div className="finished-detail-pane-summary">
          <span>{editPlan.length || beats.length} 个镜头</span>
          <span>{pendingReplacementCount > 0 ? `待应用 ${pendingReplacementCount} 项` : formatDuration(work.duration_ms)}</span>
        </div>
        <div className="finished-detail-plan-list">
          {editPlan.map((clip, index) => {
            const pending = pendingReplacements[clip.id];
            return (
              <article
                className={`finished-detail-plan-row${selectedClipID === clip.id ? " is-active" : ""}${pending ? " is-modified" : ""}`}
                key={clip.id}
                role="button"
                tabIndex={0}
                onClick={() => selectClip(clip)}
                onKeyDown={(event) => {
                  if (event.key === "Enter" || event.key === " ") {
                    event.preventDefault();
                    selectClip(clip);
                  }
                }}
              >
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
                  {pending ? (
                    <small className="finished-detail-plan-replacement">
                      替换为 {pending.candidate.asset_name || pending.candidate.file_name}
                      {pending.candidate.shortfall_ms ? ` · 提前转场 ${pending.candidate.shortfall_ms}ms` : ""}
                    </small>
                  ) : null}
                </span>
                <span className="finished-detail-plan-tags">
                  {clipPositions[index].total > 1 ? <Tag color="cyan">同段 {clipPositions[index].position}/{clipPositions[index].total}</Tag> : null}
                  <Tag>{sourceTypeLabels[clip.source_type]}</Tag>
                  {pending ? <Tag color="gold">已修改</Tag> : null}
                  {clip.use_original_audio ? <Tag color="blue">原声</Tag> : null}
                  {pending ? (
                    <Tooltip title="恢复当前素材">
                      <Button
                        type="text"
                        size="small"
                        aria-label="恢复当前素材"
                        icon={<Undo2 size={14} />}
                        onClick={(event) => {
                          event.stopPropagation();
                          restoreClipMaterial(clip.id);
                        }}
                      />
                    </Tooltip>
                  ) : null}
                  <Button
                    size="small"
                    icon={<RefreshCw size={13} />}
                    disabled={work.status !== "completed" || applyingReplacements}
                    onClick={(event) => {
                      event.stopPropagation();
                      openClipReplacement(clip);
                    }}
                  >
                    替换
                  </Button>
                </span>
              </article>
            );
          })}
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
      </section>

      {replacementClip ? (
        <section className="finished-detail-candidate-panel" aria-label="替换镜头素材">
          <header className="finished-detail-candidate-header">
            <div>
              <strong>替换镜头 {String(editPlan.findIndex((clip) => clip.id === replacementClip.id) + 1).padStart(2, "0")}</strong>
              <span>{replacementClip.label} · {formatDuration(replacementClip.end_ms - replacementClip.start_ms)}</span>
            </div>
            <Tooltip title="关闭候选素材">
              <Button type="text" aria-label="关闭候选素材" icon={<X size={16} />} onClick={closeClipReplacement} />
            </Tooltip>
          </header>

          <div className="finished-detail-candidate-search">
            <Input.Search
              value={candidateQuery}
              prefix={<Search size={14} />}
              placeholder="搜索画面或动作"
              enterButton="搜索"
              loading={candidateLoading}
              onChange={(event) => setCandidateQuery(event.target.value)}
              onSearch={(value) => void loadClipCandidates(replacementClip, value)}
            />
            {candidateData ? (
              <button
                type="button"
                className={`finished-detail-current-candidate${previewCandidate?.asset_id === candidateData.current.asset_id ? " is-selected" : ""}`}
                onClick={() => selectCandidate(candidateData.current)}
              >
                <span>当前素材</span>
                <strong>{candidateData.current.asset_name || candidateData.current.file_name}</strong>
                {candidateData.current.semantic_score !== undefined ? <b>{(candidateData.current.semantic_score * 100).toFixed(1)}</b> : null}
              </button>
            ) : null}
          </div>

          <div className="finished-detail-candidate-results">
            {candidateLoading && !candidateData ? (
              <div className="finished-detail-candidate-state"><LoaderCircle size={18} />正在检索候选素材</div>
            ) : candidateError ? (
              <div className="finished-detail-candidate-state is-error">
                <CircleAlert size={18} />
                <span>{candidateError}</span>
                <Button size="small" onClick={() => void loadClipCandidates(replacementClip, candidateQuery)}>重试</Button>
              </div>
            ) : displayedCandidates.length === 0 ? (
              <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="没有语义检索结果" />
            ) : displayedCandidates.map((candidate) => {
              const pendingElsewhere = pendingAssetIDs.has(candidate.asset_id);
              const unavailableReason = pendingElsewhere ? "已在其它待应用镜头中使用" : candidate.unavailable_reason;
              const selectable = candidate.selectable && !pendingElsewhere;
              return (
                <button
                  type="button"
                  className={`finished-detail-candidate-row${previewCandidate?.asset_id === candidate.asset_id ? " is-selected" : ""}${selectable ? "" : " is-unavailable"}`}
                  key={candidate.asset_id}
                  onClick={() => selectCandidate(candidate)}
                >
                  <span className="finished-detail-candidate-thumb">
                    {candidate.thumbnail_url ? <img src={candidate.thumbnail_url} alt="" /> : <MonitorPlay size={20} />}
                    <small>{formatDuration(candidate.duration_ms)}</small>
                  </span>
                  <span className="finished-detail-candidate-copy">
                    <strong>{candidate.asset_name || candidate.file_name}</strong>
                    <span>{candidate.action_description || candidate.scene_description || "暂无动作描述"}</span>
                    {unavailableReason ? (
                      <small className="is-unavailable">{unavailableReason}</small>
                    ) : candidate.shortfall_ms ? (
                      <small className="is-transition">提前切入下一镜头 {candidate.shortfall_ms}ms</small>
                    ) : candidate.action_description && candidate.scene_description ? <small>{candidate.scene_description}</small> : null}
                  </span>
                  <span className="finished-detail-candidate-meta">
                    {!selectable ? <Tag color="red">不可用</Tag> : candidate.shortfall_ms ? <Tag color="gold">提前转场</Tag> : null}
                    {candidate.semantic_score !== undefined ? <b>{(candidate.semantic_score * 100).toFixed(1)}</b> : null}
                  </span>
                </button>
              );
            })}
          </div>

          {previewCandidate && candidateData ? (
            <footer className="finished-detail-candidate-footer">
              <div className="finished-detail-candidate-selection">
                <strong>{previewCandidate.asset_name || previewCandidate.file_name}</strong>
                <span>
                  取用 {formatTimestamp(candidateSourceInMs)} - {formatTimestamp(candidateSourceInMs + previewSourceDurationMs)}
                  {previewShortfallMs > 0 ? ` · 提前转场 ${previewShortfallMs}ms` : ""}
                </span>
                {previewUnavailableReason ? <small>{previewUnavailableReason}</small> : null}
              </div>
              <Slider
                min={0}
                max={previewCandidate.max_source_in_ms}
                step={100}
                value={candidateSourceInMs}
                disabled={previewCandidate.max_source_in_ms <= 0}
                tooltip={{ formatter: (value) => formatTimestamp(value ?? 0) }}
                onChange={setCandidateSourceInMs}
              />
              <Button
                type="primary"
                icon={<Check size={15} />}
                disabled={!previewSelectable || previewMatchesPending || (previewMatchesCurrent && !replacementPending)}
                onClick={stageCandidate}
              >
                {!previewSelectable
                  ? "不可选"
                  : previewMatchesPending
                  ? "已选用"
                  : previewMatchesCurrent && replacementPending
                    ? "恢复当前素材"
                    : previewMatchesCurrent
                      ? "当前素材"
                      : "使用此素材"}
              </Button>
            </footer>
          ) : null}
        </section>
      ) : null}

      {pendingReplacementCount > 0 ? (
        <footer className="finished-detail-plan-actions">
          <span>已修改 {pendingReplacementCount} 个镜头</span>
          <Button
            onClick={() => {
              setPendingReplacements({});
              setPendingPlanUpdatedAt("");
              if (candidateData) {
                setPreviewCandidate(candidateData.current);
                setCandidateSourceInMs(candidateData.current.source_in_ms);
              }
            }}
            disabled={applyingReplacements}
          >
            放弃修改
          </Button>
          <Button type="primary" loading={applyingReplacements} onClick={() => void applyClipReplacements()}>
            应用并重新渲染
          </Button>
        </footer>
      ) : null}
    </div>
  );

  return (
    <div className="finished-detail-page" data-testid="finished-work-detail">
      <header className="finished-detail-header">
        <Tooltip title="返回成品库">
          <Button type="text" aria-label="返回成品库" icon={<ArrowLeft size={18} />} onClick={() => runAfterPendingDecision(onBack)} />
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
          {!isFailed ? (
            <Button
              icon={<Mic2 size={15} />}
              loading={voiceoverAction === "generate"}
              disabled={isGenerating || actionBusy || pendingReplacementCount > 0}
              onClick={() => void generateVoiceoverReplacement()}
            >
              重新生成配音
            </Button>
          ) : null}
          <Button
            icon={<RotateCcw size={15} />}
            loading={actionKind === "retry" || actionKind === "regenerate"}
            disabled={isGenerating || actionBusy || pendingReplacementCount > 0}
            onClick={isFailed ? onRetry : onRegenerate}
          >
            {isFailed ? "重试" : "重新生成"}
          </Button>
          <Button
            danger
            icon={<Trash2 size={15} />}
            loading={actionKind === "delete"}
            disabled={isGenerating || actionBusy || pendingReplacementCount > 0}
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
              className={`finished-detail-preview${isGenerating && !work.video_url ? " is-generating" : ""}`}
              ref={previewRef}
              onWheel={handlePreviewWheel}
            >
              {previewMode === "candidate" && previewCandidate ? (
                <video
                  key={previewCandidate.asset_id}
                  src={previewCandidate.video_url}
                  controls
                  muted
                  playsInline
                  preload="metadata"
                  aria-label={`${previewCandidate.asset_name || previewCandidate.file_name}候选素材`}
                />
              ) : (
                <FinishedWorkVisual key={work.id} work={work} />
              )}
              {!work.video_url ? <span className="finished-detail-preview-scrim" /> : null}
              {previewCandidate ? (
                <Segmented<"work" | "candidate">
                  className="finished-detail-preview-mode"
                  size="small"
                  value={previewMode}
                  options={[
                    { value: "work", label: "成片片段" },
                    { value: "candidate", label: "候选素材" }
                  ]}
                  onChange={setPreviewMode}
                />
              ) : null}
              {isGenerating && !work.video_url ? (
                <span className="finished-detail-generation-state">
                  <LoaderCircle size={22} />
                  <span>{work.stage_label}</span>
                </span>
              ) : isFailed ? (
                <span className="finished-detail-generation-state is-failed">
                  <CircleAlert size={22} />
                  <span>{work.error_message || "生成失败"}</span>
                </span>
              ) : work.video_url || previewMode === "candidate" ? null : (
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
                {isGenerating && work.video_url ? "正在应用镜头修改，当前播放原成片" : work.stage_label}
              </span>
              <span>{selectedClip ? `${formatTimestamp(selectedClip.start_ms)} - ${formatTimestamp(selectedClip.end_ms)}` : formatDuration(work.duration_ms)}</span>
            </div>
            {isGenerating ? <Progress percent={work.progress} showInfo={false} size="small" strokeColor="#4fc1b2" /> : null}
            {work.audio_url && !work.video_url ? <audio className="finished-detail-audio" controls preload="metadata" src={work.audio_url} aria-label="旁白音频" /> : null}
          </section>

          <aside className="finished-detail-inspector" aria-label="成品信息">
            <Tabs
              size="small"
              activeKey={activeTab}
              onChange={setActiveTab}
              items={[
                { key: "overview", label: <span className="finished-detail-tab-label"><FileText size={14} />概览</span>, children: overviewPane },
                { key: "captions", label: <span className="finished-detail-tab-label"><Captions size={14} />字幕 <b>{narrationSegments.length}</b></span>, children: captionPane },
                { key: "plan", label: <span className="finished-detail-tab-label"><Clapperboard size={14} />镜头编排 <b>{editPlan.length || beats.length}</b></span>, children: planPane }
              ]}
            />
          </aside>
        </div>
      </main>
      <Modal
        className="voiceover-replacement-modal"
        title="重新生成配音"
        open={voiceoverModalOpen}
        width={440}
        centered
        closable={voiceoverReplacement?.status !== "applying"}
        maskClosable={false}
        onCancel={() => void closeVoiceoverReplacement()}
        footer={[
          <Button key="cancel" loading={voiceoverAction === "cancel"} disabled={voiceoverReplacement?.status === "applying"} onClick={() => void closeVoiceoverReplacement()}>
            取消
          </Button>,
          <Button key="again" icon={<RefreshCw size={15} />} loading={voiceoverAction === "generate"} disabled={voiceoverReplacement?.status === "applying"} onClick={() => void generateVoiceoverReplacement()}>
            再生成一次
          </Button>,
          <Button key="apply" type="primary" loading={voiceoverAction === "apply" || voiceoverReplacement?.status === "applying"} disabled={voiceoverReplacement?.status !== "ready"} onClick={() => void applyGeneratedVoiceover()}>
            应用
          </Button>
        ]}
      >
        <div className="voiceover-replacement-content">
          {voiceoverReplacement?.status === "generating" || !voiceoverReplacement ? (
            <div className="voiceover-replacement-progress"><LoaderCircle size={20} /><span>正在生成并检查旁白...</span></div>
          ) : null}
          {voiceoverReplacement?.status === "applying" ? (
            <div className="voiceover-replacement-progress"><LoaderCircle size={20} /><span>正在按原镜头重新渲染...</span></div>
          ) : null}
          {voiceoverReplacement?.status === "failed" ? (
            <Alert type="error" showIcon message="配音处理失败" description={voiceoverReplacement.error_message || "请重新生成后再试"} />
          ) : null}
          {voiceoverReplacement?.status === "ready" && voiceoverReplacement.audio_url ? (
            <>
              <div className="voiceover-replacement-meta">
                <span>当前音色：{work.voice_profile_name || "原音色"}</span>
                <span>{formatDuration(voiceoverReplacement.duration_ms ?? 0)}</span>
              </div>
              <audio controls preload="metadata" src={voiceoverReplacement.audio_url} aria-label="新旁白试听" />
            </>
          ) : null}
        </div>
      </Modal>
    </div>
  );
}
