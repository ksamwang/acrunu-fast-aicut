import React, { useEffect, useMemo, useRef, useState } from "react";
import { Button, Space, Typography } from "antd";

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
  analysisOverlay?: React.ReactNode;
  onTrimChange: (range: TrimRange) => void;
};

const MIN_TRIM_FRAMES = 1;

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

export function VideoTrimEditor({
  src,
  durationMs = 0,
  fps,
  trimInMs,
  trimOutMs,
  analysisOverlay,
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
    await video.play();
    setIsPlaying(true);
  };

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
            <Button size="small" onClick={() => void togglePlay()}>
              {isPlaying ? "暂停" : "播放"}
            </Button>
            <Button size="small" onClick={() => seekToFrame(inFrame)}>
              到起点
            </Button>
            <Button size="small" onClick={() => seekToFrame(outFrame)}>
              到终点
            </Button>
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
