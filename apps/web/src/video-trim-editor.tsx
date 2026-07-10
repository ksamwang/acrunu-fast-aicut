import React, { useEffect, useMemo, useRef, useState } from "react";
import { Button, Space, Tooltip, Typography } from "antd";

type TrimRange = {
  inFrame: number;
  outFrame: number;
  inMs: number;
  outMs: number;
};

type DragTarget = "playhead" | "in" | "out";

type VideoTrimEditorProps = {
  src: string;
  durationMs?: number;
  fps?: number;
  trimInMs: number;
  trimOutMs: number;
  hotkeysEnabled?: boolean;
  analysisOverlay?: React.ReactNode;
  extraControls?: React.ReactNode;
  onTrimChange: (range: TrimRange) => void;
};

const MIN_TRIM_FRAMES = 1;
const RULER_INTERVALS_SECONDS = [0.5, 1, 2, 3, 5, 10, 15, 30, 60, 120, 300];

function normalizeFps(fps?: number) {
  return fps && Number.isFinite(fps) && fps > 0 ? fps : 30;
}

function clamp(value: number, min: number, max: number) {
  return Math.min(Math.max(value, min), max);
}

function frameToMs(frame: number, fps: number) {
  return Math.round((frame / fps) * 1000);
}

function msToFrame(ms: number, fps: number, totalFrames: number) {
  return clamp(Math.round((Math.max(ms, 0) / 1000) * fps), 0, totalFrames);
}

function formatSeconds(seconds: number) {
  if (!Number.isFinite(seconds)) {
    return "0.000s";
  }
  const minutes = Math.floor(seconds / 60);
  const rest = seconds - minutes * 60;
  if (minutes <= 0) {
    return `${rest.toFixed(3)}s`;
  }
  return `${minutes}:${rest.toFixed(3).padStart(6, "0")}`;
}

function SvgIcon({ children }: { children: React.ReactNode }) {
  return (
    <svg className="video-trim-icon" viewBox="0 0 24 24" aria-hidden="true" focusable="false">
      {children}
    </svg>
  );
}

function PlayIcon() {
  return (
    <SvgIcon>
      <path d="M8 5v14l11-7z" />
    </SvgIcon>
  );
}

function PauseIcon() {
  return (
    <SvgIcon>
      <path d="M8 5v14" />
      <path d="M16 5v14" />
    </SvgIcon>
  );
}

function JumpToInIcon() {
  return (
    <SvgIcon>
      <path d="M7 5v14" />
      <path d="M18 7l-7 5 7 5z" />
    </SvgIcon>
  );
}

function JumpToOutIcon() {
  return (
    <SvgIcon>
      <path d="M17 5v14" />
      <path d="M6 7l7 5-7 5z" />
    </SvgIcon>
  );
}

function isEditableKeyboardTarget(target: EventTarget | null) {
  if (!(target instanceof Element)) {
    return false;
  }
  return Boolean(
    target.closest(
      'input, textarea, select, button, a, [contenteditable="true"], [role="button"], .ant-btn, .ant-select, .ant-select-dropdown, .ant-picker-dropdown'
    )
  );
}

function chooseRulerInterval(durationSeconds: number) {
  if (!Number.isFinite(durationSeconds) || durationSeconds <= 0) {
    return 1;
  }
  const idealInterval = durationSeconds / 6;
  return RULER_INTERVALS_SECONDS.find((interval) => interval >= idealInterval) ?? RULER_INTERVALS_SECONDS[RULER_INTERVALS_SECONDS.length - 1];
}

function buildRulerTicks(durationSeconds: number) {
  if (!Number.isFinite(durationSeconds) || durationSeconds <= 0) {
    return [{ seconds: 0, percent: 0, label: formatSeconds(0) }];
  }

  const interval = chooseRulerInterval(durationSeconds);
  const ticks = [];
  for (let seconds = 0; seconds < durationSeconds; seconds += interval) {
    ticks.push({
      seconds,
      percent: clamp((seconds / durationSeconds) * 100, 0, 100),
      label: formatSeconds(seconds)
    });
  }

  const lastTick = ticks[ticks.length - 1];
  if (!lastTick || Math.abs(lastTick.seconds - durationSeconds) > 0.001) {
    const endTick = {
      seconds: durationSeconds,
      percent: 100,
      label: formatSeconds(durationSeconds)
    };
    if (lastTick && durationSeconds - lastTick.seconds < interval * 0.35) {
      ticks[ticks.length - 1] = endTick;
    } else {
      ticks.push(endTick);
    }
  }
  return ticks;
}

export function VideoTrimEditor({
  src,
  durationMs = 0,
  fps,
  trimInMs,
  trimOutMs,
  hotkeysEnabled = false,
  analysisOverlay,
  extraControls,
  onTrimChange
}: VideoTrimEditorProps) {
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const timelineRef = useRef<HTMLDivElement | null>(null);
  const dragTargetRef = useRef<DragTarget | null>(null);
  const [isPlaying, setIsPlaying] = useState(false);
  const [currentFrame, setCurrentFrame] = useState(0);
  const [volume, setVolume] = useState(1);
  const [mediaDurationMs, setMediaDurationMs] = useState(0);

  const frameRate = useMemo(() => normalizeFps(fps), [fps]);
  const effectiveDurationMs = durationMs > 0 ? durationMs : mediaDurationMs;
  const totalFrames = useMemo(
    () => Math.max(1, Math.round((Math.max(effectiveDurationMs, 0) / 1000) * frameRate)),
    [effectiveDurationMs, frameRate]
  );
  const inFrame = msToFrame(trimInMs, frameRate, totalFrames);
  const outFrame = clamp(msToFrame(trimOutMs || effectiveDurationMs, frameRate, totalFrames), inFrame + MIN_TRIM_FRAMES, totalFrames);
  const safeCurrentFrame = clamp(currentFrame, 0, totalFrames);

  const emitTrim = (nextInFrame: number, nextOutFrame: number) => {
    const normalizedInFrame = clamp(nextInFrame, 0, totalFrames - MIN_TRIM_FRAMES);
    const normalizedOutFrame = clamp(nextOutFrame, normalizedInFrame + MIN_TRIM_FRAMES, totalFrames);
    onTrimChange({
      inFrame: normalizedInFrame,
      outFrame: normalizedOutFrame,
      inMs: frameToMs(normalizedInFrame, frameRate),
      outMs: frameToMs(normalizedOutFrame, frameRate)
    });
  };

  const seekToFrame = (frame: number) => {
    const nextFrame = clamp(frame, 0, totalFrames);
    setCurrentFrame(nextFrame);
    if (videoRef.current) {
      videoRef.current.currentTime = nextFrame / frameRate;
    }
  };

  const currentPlaybackFrame = () => {
    const video = videoRef.current;
    if (video) {
      return msToFrame(video.currentTime * 1000, frameRate, totalFrames);
    }
    return safeCurrentFrame;
  };

  useEffect(() => {
    if (safeCurrentFrame < inFrame || safeCurrentFrame > outFrame) {
      seekToFrame(inFrame);
    }
  }, [inFrame, outFrame]);

  useEffect(() => {
    if (!isPlaying) {
      return;
    }

    let animationFrame = 0;
    const tick = () => {
      const video = videoRef.current;
      if (video) {
        const frame = msToFrame(video.currentTime * 1000, frameRate, totalFrames);
        setCurrentFrame(frame);
        if (frame >= outFrame) {
          video.pause();
          setIsPlaying(false);
          seekToFrame(outFrame);
          return;
        }
      }
      animationFrame = window.requestAnimationFrame(tick);
    };

    animationFrame = window.requestAnimationFrame(tick);
    return () => window.cancelAnimationFrame(animationFrame);
  }, [frameRate, isPlaying, outFrame, totalFrames]);

  const frameFromPointer = (clientX: number) => {
    const rect = timelineRef.current?.getBoundingClientRect();
    if (!rect || rect.width <= 0) {
      return 0;
    }
    return clamp(Math.round(((clientX - rect.left) / rect.width) * totalFrames), 0, totalFrames);
  };

  const applyDrag = (target: DragTarget, frame: number) => {
    if (target === "playhead") {
      seekToFrame(frame);
      return;
    }
    if (target === "in") {
      const nextInFrame = clamp(frame, 0, outFrame - MIN_TRIM_FRAMES);
      emitTrim(nextInFrame, outFrame);
      seekToFrame(nextInFrame);
      return;
    }
    const nextOutFrame = clamp(frame, inFrame + MIN_TRIM_FRAMES, totalFrames);
    emitTrim(inFrame, nextOutFrame);
    seekToFrame(nextOutFrame);
  };

  const startDrag = (target: DragTarget, event: React.PointerEvent<HTMLElement>) => {
    event.preventDefault();
    event.stopPropagation();
    dragTargetRef.current = target;
    event.currentTarget.setPointerCapture(event.pointerId);
    const video = videoRef.current;
    if (video && !video.paused) {
      video.pause();
      setIsPlaying(false);
    }
    applyDrag(target, frameFromPointer(event.clientX));

    const handleWindowPointerMove = (moveEvent: PointerEvent) => {
      moveEvent.preventDefault();
      applyDrag(target, frameFromPointer(moveEvent.clientX));
    };
    const handleWindowPointerUp = () => {
      dragTargetRef.current = null;
      window.removeEventListener("pointermove", handleWindowPointerMove);
      window.removeEventListener("pointerup", handleWindowPointerUp);
      window.removeEventListener("pointercancel", handleWindowPointerUp);
    };

    window.addEventListener("pointermove", handleWindowPointerMove);
    window.addEventListener("pointerup", handleWindowPointerUp);
    window.addEventListener("pointercancel", handleWindowPointerUp);
  };

  const startMouseDrag = (target: DragTarget, event: React.MouseEvent<HTMLElement>) => {
    if (dragTargetRef.current) {
      return;
    }
    event.preventDefault();
    event.stopPropagation();
    dragTargetRef.current = target;
    const video = videoRef.current;
    if (video && !video.paused) {
      video.pause();
      setIsPlaying(false);
    }
    applyDrag(target, frameFromPointer(event.clientX));

    const handleWindowMouseMove = (moveEvent: MouseEvent) => {
      moveEvent.preventDefault();
      applyDrag(target, frameFromPointer(moveEvent.clientX));
    };
    const handleWindowMouseUp = () => {
      dragTargetRef.current = null;
      window.removeEventListener("mousemove", handleWindowMouseMove);
      window.removeEventListener("mouseup", handleWindowMouseUp);
    };

    window.addEventListener("mousemove", handleWindowMouseMove);
    window.addEventListener("mouseup", handleWindowMouseUp);
  };

  const updateDrag = (event: React.PointerEvent<HTMLElement>) => {
    const target = dragTargetRef.current;
    if (!target) {
      return;
    }
    event.preventDefault();
    applyDrag(target, frameFromPointer(event.clientX));
  };

  const endDrag = (event: React.PointerEvent<HTMLElement>) => {
    if (dragTargetRef.current) {
      event.currentTarget.releasePointerCapture(event.pointerId);
    }
    dragTargetRef.current = null;
  };

  const togglePlay = async () => {
    const video = videoRef.current;
    if (!video) {
      return;
    }
    if (!video.paused) {
      video.pause();
      setIsPlaying(false);
      return;
    }
    if (safeCurrentFrame < inFrame || safeCurrentFrame >= outFrame) {
      seekToFrame(inFrame);
    }
    try {
      await video.play();
      setIsPlaying(true);
    } catch {
      setIsPlaying(false);
    }
  };

  const setInPointAtCurrentFrame = () => {
    const nextInFrame = clamp(currentPlaybackFrame(), 0, outFrame - MIN_TRIM_FRAMES);
    emitTrim(nextInFrame, outFrame);
  };

  const setOutPointAtCurrentFrame = () => {
    const nextOutFrame = clamp(currentPlaybackFrame(), inFrame + MIN_TRIM_FRAMES, totalFrames);
    emitTrim(inFrame, nextOutFrame);
  };

  useEffect(() => {
    if (!hotkeysEnabled) {
      return;
    }

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.repeat || isEditableKeyboardTarget(event.target)) {
        return;
      }
      if (event.code === "Space") {
        event.preventDefault();
        void togglePlay();
        return;
      }
      const key = event.key.toLowerCase();
      if (key === "i") {
        event.preventDefault();
        setInPointAtCurrentFrame();
        return;
      }
      if (key === "o") {
        event.preventDefault();
        setOutPointAtCurrentFrame();
      }
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [hotkeysEnabled, inFrame, outFrame, safeCurrentFrame, totalFrames, frameRate]);

  const handleTimeUpdate = () => {
    const video = videoRef.current;
    if (!video) {
      return;
    }
    setCurrentFrame(msToFrame(video.currentTime * 1000, frameRate, totalFrames));
  };

  const handleLoadedMetadata = () => {
    const video = videoRef.current;
    if (video?.duration && Number.isFinite(video.duration)) {
      setMediaDurationMs(Math.round(video.duration * 1000));
    }
  };

  const handleVolumeChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    const nextVolume = Number(event.target.value);
    setVolume(nextVolume);
    if (videoRef.current) {
      videoRef.current.volume = nextVolume;
    }
  };

  const requestFullscreen = () => {
    void videoRef.current?.parentElement?.requestFullscreen?.();
  };

  const inPercent = (inFrame / totalFrames) * 100;
  const outPercent = (outFrame / totalFrames) * 100;
  const playheadPercent = (safeCurrentFrame / totalFrames) * 100;
  const selectedFrameCount = Math.max(0, outFrame - inFrame);
  const rulerTicks = useMemo(() => buildRulerTicks(totalFrames / frameRate), [frameRate, totalFrames]);

  return (
    <div className="video-trim-editor">
      <div className="video-trim-stage">
        <video
          ref={videoRef}
          className="video-trim-player"
          src={src}
          controls={false}
          onLoadedMetadata={handleLoadedMetadata}
          onTimeUpdate={handleTimeUpdate}
          onPause={() => setIsPlaying(false)}
          onPlay={() => setIsPlaying(true)}
        />
        {analysisOverlay}
      </div>

      <div className="video-trim-control-panel">
        <div className="video-trim-toolbar">
          <Space size="small" wrap>
            <Tooltip title={isPlaying ? "暂停（Space）" : "播放（Space）"}>
              <Button
                size="small"
                className="video-trim-icon-button"
                aria-label={isPlaying ? "暂停" : "播放"}
                icon={isPlaying ? <PauseIcon /> : <PlayIcon />}
                onClick={() => void togglePlay()}
              />
            </Tooltip>
            <Tooltip title="跳到入点">
              <Button
                size="small"
                className="video-trim-icon-button"
                aria-label="跳到入点"
                icon={<JumpToInIcon />}
                onClick={() => seekToFrame(inFrame)}
              />
            </Tooltip>
            <Tooltip title="跳到出点">
              <Button
                size="small"
                className="video-trim-icon-button"
                aria-label="跳到出点"
                icon={<JumpToOutIcon />}
                onClick={() => seekToFrame(outFrame)}
              />
            </Tooltip>
            {extraControls}
          </Space>
          <Typography.Text className="video-trim-timecode">
            {formatSeconds(safeCurrentFrame / frameRate)} / {formatSeconds(totalFrames / frameRate)}
          </Typography.Text>
          <Space size="small" className="video-trim-right-tools">
            <Typography.Text type="secondary">音量</Typography.Text>
            <input className="video-trim-volume" type="range" min={0} max={1} step={0.05} value={volume} onChange={handleVolumeChange} />
            <Button size="small" onClick={requestFullscreen}>
              全屏
            </Button>
          </Space>
        </div>

        <div
          ref={timelineRef}
          className="video-trim-timeline"
          onPointerDown={(event) => startDrag("playhead", event)}
          onMouseDown={(event) => startMouseDrag("playhead", event)}
          onPointerMove={updateDrag}
          onPointerUp={endDrag}
          onPointerCancel={endDrag}
        >
          <div className="video-trim-ruler" aria-hidden="true">
            {rulerTicks.map((tick) => (
              <div key={`${tick.seconds}-${tick.percent}`} className="video-trim-ruler-tick" style={{ left: `${tick.percent}%` }}>
                <span className="video-trim-ruler-line" />
                <span className="video-trim-ruler-label">{tick.label}</span>
              </div>
            ))}
          </div>
          <div className="video-trim-track" />
          <div className="video-trim-selection" style={{ left: `${inPercent}%`, width: `${Math.max(outPercent - inPercent, 0)}%` }} />
          <button
            type="button"
            className="video-trim-handle video-trim-handle-in"
            style={{ left: `${inPercent}%` }}
            aria-label="裁切起点"
            onPointerDown={(event) => startDrag("in", event)}
            onMouseDown={(event) => startMouseDrag("in", event)}
            onPointerMove={updateDrag}
            onPointerUp={endDrag}
            onPointerCancel={endDrag}
          />
          <button
            type="button"
            className="video-trim-handle video-trim-handle-out"
            style={{ left: `${outPercent}%` }}
            aria-label="裁切终点"
            onPointerDown={(event) => startDrag("out", event)}
            onMouseDown={(event) => startMouseDrag("out", event)}
            onPointerMove={updateDrag}
            onPointerUp={endDrag}
            onPointerCancel={endDrag}
          />
          <div className="video-trim-playhead" style={{ left: `${playheadPercent}%` }} />
        </div>

        <div className="video-trim-readout">
          <Typography.Text>起点：第 {inFrame} 帧 / {formatSeconds(inFrame / frameRate)}</Typography.Text>
          <Typography.Text>终点：第 {outFrame} 帧 / {formatSeconds(outFrame / frameRate)}</Typography.Text>
          <Typography.Text>片段：{selectedFrameCount} 帧 / {formatSeconds(selectedFrameCount / frameRate)}</Typography.Text>
          <Typography.Text>帧率：{frameRate.toFixed(3)} fps</Typography.Text>
        </div>
      </div>
    </div>
  );
}
