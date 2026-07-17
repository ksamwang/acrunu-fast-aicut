import { useEffect, useMemo, useRef, useState } from "react";
import { Button, Empty, Input, InputNumber, Modal, Popconfirm, Segmented, Select, Switch, Tag, Tooltip, Upload, message } from "antd";
import { Archive, Music2, Pause, Pencil, Play, Plus, Search, Upload as UploadIcon } from "lucide-react";
import { formatDuration } from "../../shared/lib/format";
import type { BGMTrack, BGMTrackInput } from "../../shared/types/bgm";
import { archiveBGMTrack, createBGMTrack, listBGMTracks, updateBGMTrack } from "./api";
import "./styles.css";

const emptyInput: BGMTrackInput = { name: "", bpm: 0, mood: "", tags: [], status: "enabled" };

function formatFileSize(bytes: number) {
  if (bytes < 1024 * 1024) {
    return `${Math.max(1, Math.round(bytes / 1024))} KB`;
  }
  return `${(bytes / 1024 / 1024).toFixed(bytes >= 10 * 1024 * 1024 ? 0 : 1)} MB`;
}

function cleanTags(value: string) {
  return Array.from(new Set(value.split(/[，,\n]/).map((item) => item.trim()).filter(Boolean))).slice(0, 20);
}

export function BGMLibraryPage({ token }: { token: string }) {
  const [tracks, setTracks] = useState<BGMTrack[]>([]);
  const [loading, setLoading] = useState(true);
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState<"all" | BGMTrack["status"]>("all");
  const [mood, setMood] = useState("all");
  const [editing, setEditing] = useState<BGMTrack | null>(null);
  const [form, setForm] = useState<BGMTrackInput>(emptyInput);
  const [tagsText, setTagsText] = useState("");
  const [uploadOpen, setUploadOpen] = useState(false);
  const [uploadFile, setUploadFile] = useState<File | null>(null);
  const [saving, setSaving] = useState(false);
  const [updatingID, setUpdatingID] = useState("");
  const [activeTrack, setActiveTrack] = useState<BGMTrack | null>(null);
  const [playing, setPlaying] = useState(false);
  const audioRef = useRef<HTMLAudioElement | null>(null);

  const refresh = async () => {
    setLoading(true);
    try {
      setTracks(await listBGMTracks("/api/bgm-tracks", token, true));
    } catch (error) {
      message.error(error instanceof Error ? error.message : "音乐列表加载失败");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void refresh();
  }, [token]);

  useEffect(() => () => audioRef.current?.pause(), []);

  const moodOptions = useMemo(() => [
    { value: "all", label: "全部情绪" },
    ...Array.from(new Set(tracks.map((track) => track.mood).filter(Boolean))).map((value) => ({ value: value!, label: value! }))
  ], [tracks]);

  const filteredTracks = useMemo(() => {
    const keyword = query.trim().toLocaleLowerCase();
    return tracks.filter((track) => {
      const matchesStatus = status === "all" || track.status === status;
      const matchesMood = mood === "all" || track.mood === mood;
      const haystack = `${track.name} ${track.file_name} ${track.mood ?? ""} ${track.tags.join(" ")}`.toLocaleLowerCase();
      return matchesStatus && matchesMood && (!keyword || haystack.includes(keyword));
    });
  }, [mood, query, status, tracks]);

  const resetForm = () => {
    setForm(emptyInput);
    setTagsText("");
    setUploadFile(null);
    setEditing(null);
  };

  const openUpload = () => {
    resetForm();
    setUploadOpen(true);
  };

  const openEdit = (track: BGMTrack) => {
    setEditing(track);
    setForm({ name: track.name, bpm: track.bpm ?? 0, mood: track.mood ?? "", tags: track.tags, status: track.status === "enabled" ? "enabled" : "disabled" });
    setTagsText(track.tags.join("，"));
  };

  const saveUpload = async () => {
    if (!form.name.trim() || !uploadFile) {
      message.warning("请填写名称并选择音乐文件");
      return;
    }
    setSaving(true);
    try {
      const created = await createBGMTrack({ ...form, tags: cleanTags(tagsText) }, uploadFile, token);
      setTracks((current) => [created, ...current]);
      setUploadOpen(false);
      resetForm();
      message.success("音乐已入库");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "音乐上传失败");
    } finally {
      setSaving(false);
    }
  };

  const saveEdit = async () => {
    if (!editing || !form.name.trim()) {
      return;
    }
    setSaving(true);
    try {
      const updated = await updateBGMTrack(editing.id, { ...form, tags: cleanTags(tagsText) }, token);
      setTracks((current) => current.map((track) => track.id === updated.id ? updated : track));
      setEditing(null);
      message.success("音乐信息已保存");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "音乐信息保存失败");
    } finally {
      setSaving(false);
    }
  };

  const toggleStatus = async (track: BGMTrack, enabled: boolean) => {
    setUpdatingID(track.id);
    try {
      const updated = await updateBGMTrack(track.id, {
        name: track.name, bpm: track.bpm ?? 0, mood: track.mood ?? "", tags: track.tags, status: enabled ? "enabled" : "disabled"
      }, token);
      setTracks((current) => current.map((item) => item.id === updated.id ? updated : item));
    } catch (error) {
      message.error(error instanceof Error ? error.message : "状态更新失败");
    } finally {
      setUpdatingID("");
    }
  };

  const archiveTrack = async (track: BGMTrack) => {
    setUpdatingID(track.id);
    try {
      const archived = await archiveBGMTrack(track.id, token);
      setTracks((current) => current.map((item) => item.id === archived.id ? archived : item));
      if (activeTrack?.id === track.id) {
        audioRef.current?.pause();
        setActiveTrack(null);
        setPlaying(false);
      }
      message.success("音乐已归档");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "音乐归档失败");
    } finally {
      setUpdatingID("");
    }
  };

  const togglePreview = (track: BGMTrack) => {
    const audio = audioRef.current;
    if (activeTrack?.id === track.id && audio && !audio.paused) {
      audio.pause();
      setPlaying(false);
      return;
    }
    setActiveTrack(track);
    setPlaying(true);
    window.setTimeout(() => void audioRef.current?.play().catch(() => setPlaying(false)), 0);
  };

  const editorFields = (
    <div className="bgm-form-grid">
      <label><span>名称</span><Input value={form.name} maxLength={120} onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))} /></label>
      <label><span>情绪</span><Input value={form.mood} maxLength={40} placeholder="轻快、温暖、科技" onChange={(event) => setForm((current) => ({ ...current, mood: event.target.value }))} /></label>
      <label><span>BPM</span><InputNumber value={form.bpm || null} min={20} max={300} precision={0} placeholder="可选" onChange={(value) => setForm((current) => ({ ...current, bpm: Number(value ?? 0) }))} /></label>
      <label><span>状态</span><Segmented block value={form.status} options={[{ value: "enabled", label: "启用" }, { value: "disabled", label: "停用" }]} onChange={(value) => setForm((current) => ({ ...current, status: value as BGMTrackInput["status"] }))} /></label>
      <label className="bgm-form-wide"><span>标签</span><Input value={tagsText} placeholder="使用逗号分隔" onChange={(event) => setTagsText(event.target.value)} /></label>
    </div>
  );

  return (
    <div className="bgm-page" data-testid="bgm-library-page">
      <header className="bgm-toolbar">
        <div className="bgm-search"><Search size={16} /><Input variant="borderless" value={query} allowClear placeholder="搜索名称、文件或标签" onChange={(event) => setQuery(event.target.value)} /></div>
        <Select value={mood} options={moodOptions} onChange={setMood} />
        <Segmented value={status} options={[{ value: "all", label: "全部" }, { value: "enabled", label: "启用" }, { value: "disabled", label: "停用" }, { value: "archived", label: "已归档" }]} onChange={(value) => setStatus(value as typeof status)} />
        <span className="bgm-toolbar-count">{filteredTracks.length} 首</span>
        <Button type="primary" icon={<Plus size={16} />} onClick={openUpload}>添加音乐</Button>
      </header>

      <section className="bgm-list-shell" aria-label="背景音乐列表">
        <div className="bgm-list-header" role="row">
          <span>试听</span><span>音乐</span><span>情绪 / 标签</span><span>BPM</span><span>时长</span><span>文件</span><span>启用</span><span>操作</span>
        </div>
        <div className="bgm-list-scroll">
          {filteredTracks.map((track) => (
            <div className={`bgm-list-row${track.status === "archived" ? " is-archived" : ""}`} role="row" key={track.id} data-testid={`bgm-track-${track.id}`}>
              <Button type="text" shape="circle" aria-label={`${playing && activeTrack?.id === track.id ? "暂停" : "播放"}${track.name}`} icon={playing && activeTrack?.id === track.id ? <Pause size={16} /> : <Play size={16} />} onClick={() => togglePreview(track)} />
              <span className="bgm-track-main"><strong>{track.name}</strong><small>{track.file_name}</small></span>
              <span className="bgm-track-tags">{track.mood ? <Tag color="cyan">{track.mood}</Tag> : null}{track.tags.slice(0, 3).map((tag) => <Tag key={tag}>{tag}</Tag>)}</span>
              <span className="bgm-number">{track.bpm || "-"}</span>
              <span className="bgm-number">{formatDuration(track.duration_ms)}</span>
              <span className="bgm-file-meta">{formatFileSize(track.file_size_bytes)}<small>{track.sample_rate ? `${Math.round(track.sample_rate / 1000)} kHz` : ""}</small></span>
              <span><Switch size="small" checked={track.status === "enabled"} loading={updatingID === track.id} disabled={track.status === "archived"} onChange={(checked) => void toggleStatus(track, checked)} /></span>
              <span className="bgm-row-actions">
                <Tooltip title="编辑"><Button type="text" aria-label={`编辑${track.name}`} icon={<Pencil size={15} />} disabled={track.status === "archived"} onClick={() => openEdit(track)} /></Tooltip>
                <Popconfirm title="归档这首音乐？" description="历史成品仍可使用，工作台不再提供选择。" okText="归档" cancelText="取消" onConfirm={() => void archiveTrack(track)}>
                  <Tooltip title="归档"><Button type="text" danger aria-label={`归档${track.name}`} icon={<Archive size={15} />} disabled={track.status === "archived"} /></Tooltip>
                </Popconfirm>
              </span>
            </div>
          ))}
          {!loading && filteredTracks.length === 0 ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="没有符合条件的音乐" /> : null}
          {loading ? <div className="bgm-loading"><Music2 size={18} />正在加载音乐库</div> : null}
        </div>
      </section>

      <footer className={`bgm-player${activeTrack ? " is-visible" : ""}`}>
        <span><Music2 size={15} /><strong>{activeTrack?.name ?? "未选择音乐"}</strong></span>
        <audio ref={audioRef} controls preload="metadata" src={activeTrack?.audio_url} onPlay={() => setPlaying(true)} onPause={() => setPlaying(false)} onEnded={() => setPlaying(false)} />
      </footer>

      <Modal title="添加背景音乐" open={uploadOpen} okText="上传入库" cancelText="取消" confirmLoading={saving} onOk={() => void saveUpload()} onCancel={() => { setUploadOpen(false); resetForm(); }}>
        <div className="bgm-upload-file">
          <Upload
            accept=".mp3,.wav,.m4a,.aac,.flac,.ogg"
            maxCount={1}
            fileList={uploadFile ? [{ uid: uploadFile.name, name: uploadFile.name, status: "done" }] : []}
            beforeUpload={(file) => { setUploadFile(file); setForm((current) => ({ ...current, name: current.name || file.name.replace(/\.[^.]+$/, "") })); return false; }}
            onRemove={() => { setUploadFile(null); return true; }}
          >
            <Button icon={<UploadIcon size={16} />}>选择音乐文件</Button>
          </Upload>
          <span>支持 MP3、WAV、M4A、AAC、FLAC、OGG，最大 100 MB</span>
        </div>
        {editorFields}
      </Modal>

      <Modal title="编辑背景音乐" open={Boolean(editing)} okText="保存" cancelText="取消" confirmLoading={saving} onOk={() => void saveEdit()} onCancel={() => setEditing(null)}>
        {editorFields}
      </Modal>
    </div>
  );
}
