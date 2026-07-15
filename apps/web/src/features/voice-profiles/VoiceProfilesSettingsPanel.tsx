import { Button, Empty, Form, Input, Modal, Popconfirm, Select, Space, Tag, Tooltip, Typography, Upload, message } from "antd";
import { Pause, Pencil, Play, Plus, Star, Trash2, Upload as UploadIcon } from "lucide-react";
import { useState } from "react";
import type { UploadFile } from "antd";
import type { VoiceProfile, VoiceProfileStatus } from "../../shared/types/voice";
import { createPrototypeVoiceProfileID, deletePrototypeVoiceProfile, savePrototypeVoiceProfile, setPrototypeDefaultVoiceProfile } from "./prototype-store";
import { useVoicePreview } from "./useVoicePreview";
import { useVoiceProfiles } from "./useVoiceProfiles";
import "./styles.css";

type VoiceProfileFormValues = {
  name: string;
  language: string;
  style_tags: string[];
  reference_text: string;
  preview_text: string;
  status: VoiceProfileStatus;
  is_default: boolean;
};

const maxReferenceAudioBytes = 2 * 1024 * 1024;
const defaultPreviewText = "这是一段用于确认旁白语速、语气和听感的样音。";
const styleOptions = ["自然", "亲和", "沉稳", "清晰", "轻快", "有活力", "克制", "可信"];

function readFileAsDataURL(file: File) {
  return new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onerror = () => reject(new Error("读取参考音频失败"));
    reader.onload = () => resolve(String(reader.result));
    reader.readAsDataURL(file);
  });
}

export function VoiceProfilesSettingsPanel() {
  const profiles = useVoiceProfiles();
  const { playingProfileID, togglePreview } = useVoicePreview();
  const [form] = Form.useForm<VoiceProfileFormValues>();
  const [modalOpen, setModalOpen] = useState(false);
  const [editingProfile, setEditingProfile] = useState<VoiceProfile | null>(null);
  const [referenceFile, setReferenceFile] = useState<File | null>(null);
  const [referenceFileList, setReferenceFileList] = useState<UploadFile[]>([]);
  const [referenceAudioRemoved, setReferenceAudioRemoved] = useState(false);
  const [saving, setSaving] = useState(false);

  const openCreate = () => {
    setEditingProfile(null);
    setReferenceFile(null);
    setReferenceFileList([]);
    setReferenceAudioRemoved(false);
    form.setFieldsValue({
      name: "",
      language: "中文",
      style_tags: ["自然"],
      reference_text: "",
      preview_text: defaultPreviewText,
      status: "enabled",
      is_default: profiles.every((profile) => !profile.is_default)
    });
    setModalOpen(true);
  };

  const openEdit = (profile: VoiceProfile) => {
    setEditingProfile(profile);
    setReferenceFile(null);
    setReferenceFileList(profile.reference_audio_name ? [{ uid: "existing", name: profile.reference_audio_name, status: "done" }] : []);
    setReferenceAudioRemoved(false);
    form.setFieldsValue({
      name: profile.name,
      language: profile.language,
      style_tags: profile.style_tags,
      reference_text: profile.reference_text,
      preview_text: profile.preview_text,
      status: profile.status,
      is_default: profile.is_default
    });
    setModalOpen(true);
  };

  const saveProfile = async () => {
    const values = await form.validateFields();
    if (!editingProfile && !referenceFile) {
      message.warning("请上传参考音频");
      return;
    }
    if (referenceFile && referenceFile.size > maxReferenceAudioBytes) {
      message.warning("参考音频不能超过 2MB");
      return;
    }

    setSaving(true);
    try {
      const previewAudioURL = referenceFile
        ? await readFileAsDataURL(referenceFile)
        : referenceAudioRemoved
          ? undefined
          : editingProfile?.preview_audio_url;
      const timestamp = new Date().toISOString();
      const profile: VoiceProfile = {
        id: editingProfile?.id ?? createPrototypeVoiceProfileID(),
        name: values.name.trim(),
        language: values.language,
        style_tags: values.style_tags.map((tag) => tag.trim()).filter(Boolean),
        reference_text: values.reference_text.trim(),
        preview_text: values.preview_text.trim() || defaultPreviewText,
        preview_kind: previewAudioURL ? "reference_audio" : "browser",
        preview_audio_url: previewAudioURL,
        reference_audio_name: referenceFile?.name ?? (referenceAudioRemoved ? undefined : editingProfile?.reference_audio_name),
        status: values.status,
        is_default: values.is_default,
        created_at: editingProfile?.created_at ?? timestamp,
        updated_at: timestamp
      };
      savePrototypeVoiceProfile(profile);
      setModalOpen(false);
      message.success("音色已保存");
    } catch (error) {
      message.error(error instanceof Error ? error.message : "保存音色失败");
    } finally {
      setSaving(false);
    }
  };

  const previewProfile = (profile: VoiceProfile) => {
    void togglePreview(profile).catch((error) => message.error(error instanceof Error ? error.message : "样音播放失败"));
  };

  return (
    <div className="voice-profiles-settings" data-testid="voice-profiles-settings">
      <div className="voice-profiles-settings-toolbar">
        <Typography.Text type="secondary">启用的音色可在工作台选择。</Typography.Text>
        <Button type="primary" icon={<Plus size={16} />} onClick={openCreate}>新增音色</Button>
      </div>

      {profiles.length === 0 ? (
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无音色" />
      ) : (
        <div className="voice-profiles-settings-grid">
          {profiles.map((profile) => {
            const playing = playingProfileID === profile.id;
            return (
              <article
                className={`voice-profile-settings-card${profile.status === "disabled" ? " is-disabled" : ""}`}
                data-testid={`voice-profile-settings-${profile.id}`}
                key={profile.id}
              >
                <div className="voice-profile-settings-card-heading">
                  <div>
                    <Typography.Text strong>{profile.name}</Typography.Text>
                    <Typography.Text type="secondary">{profile.language}</Typography.Text>
                  </div>
                  <div className="voice-profile-settings-card-status">
                    {profile.is_default ? <Tag color="blue">默认</Tag> : null}
                    <Tag color={profile.status === "enabled" ? "green" : "default"}>{profile.status === "enabled" ? "启用" : "停用"}</Tag>
                  </div>
                </div>
                <div className="voice-profile-settings-tags">
                  {profile.style_tags.map((tag) => <Tag key={tag}>{tag}</Tag>)}
                  <Tag>{profile.preview_kind === "reference_audio" ? "参考音频" : "原型样音"}</Tag>
                </div>
                <Typography.Paragraph className="voice-profile-settings-sample">{profile.preview_text}</Typography.Paragraph>
                <div className="voice-profile-settings-card-actions">
                  <Tooltip title={playing ? "停止试听" : "试听音色"}>
                    <Button
                      type="text"
                      aria-label={playing ? `停止试听 ${profile.name}` : `试听 ${profile.name}`}
                      icon={playing ? <Pause size={17} fill="currentColor" /> : <Play size={17} fill="currentColor" />}
                      onClick={() => previewProfile(profile)}
                    />
                  </Tooltip>
                  <Space size={4}>
                    {!profile.is_default && profile.status === "enabled" ? (
                      <Tooltip title="设为默认音色">
                        <Button type="text" aria-label={`设 ${profile.name} 为默认音色`} icon={<Star size={16} />} onClick={() => setPrototypeDefaultVoiceProfile(profile.id)} />
                      </Tooltip>
                    ) : null}
                    <Tooltip title="编辑音色">
                      <Button type="text" aria-label={`编辑 ${profile.name}`} icon={<Pencil size={16} />} onClick={() => openEdit(profile)} />
                    </Tooltip>
                    <Popconfirm title="确认删除该音色？" onConfirm={() => deletePrototypeVoiceProfile(profile.id)}>
                      <Tooltip title="删除音色">
                        <Button type="text" danger aria-label={`删除 ${profile.name}`} icon={<Trash2 size={16} />} />
                      </Tooltip>
                    </Popconfirm>
                  </Space>
                </div>
              </article>
            );
          })}
        </div>
      )}

      <Modal
        title={editingProfile ? "编辑音色" : "新增音色"}
        open={modalOpen}
        onCancel={() => setModalOpen(false)}
        onOk={() => void saveProfile()}
        confirmLoading={saving}
        destroyOnHidden
      >
        <Form form={form} layout="vertical">
          <div className="voice-profile-form-grid">
            <Form.Item name="name" label="名称" rules={[{ required: true, message: "请输入音色名称" }]}>
              <Input placeholder="例如：温和女声" />
            </Form.Item>
            <Form.Item name="language" label="语言" rules={[{ required: true, message: "请选择语言" }]}>
              <Select options={[{ value: "中文", label: "中文" }, { value: "英文", label: "英文" }, { value: "粤语", label: "粤语" }]} />
            </Form.Item>
          </div>
          <Form.Item name="style_tags" label="风格" rules={[{ required: true, message: "至少填写一个风格标签" }]}>
            <Select mode="tags" tokenSeparators={[",", "，"]} options={styleOptions.map((value) => ({ value, label: value }))} placeholder="例如：自然、亲和" />
          </Form.Item>
          <Form.Item name="reference_text" label="参考文本" rules={[{ required: true, message: "请输入参考音频对应文本" }]}>
            <Input.TextArea autoSize={{ minRows: 3, maxRows: 5 }} placeholder="填写参考音频中的准确口播文本" />
          </Form.Item>
          <Form.Item label="参考音频">
            <Upload
              accept="audio/*"
              beforeUpload={() => false}
              fileList={referenceFileList}
              maxCount={1}
              onChange={({ fileList }) => {
                const lastFile = fileList[fileList.length - 1];
                const nextFile = lastFile?.originFileObj;
                setReferenceFile(nextFile ?? null);
                setReferenceFileList(lastFile ? [lastFile] : []);
                setReferenceAudioRemoved(!lastFile && Boolean(editingProfile?.preview_audio_url));
              }}
            >
              <Button icon={<UploadIcon size={16} />}>选择音频</Button>
            </Upload>
          </Form.Item>
          <Form.Item name="preview_text" label="样音文本">
            <Input.TextArea autoSize={{ minRows: 2, maxRows: 4 }} />
          </Form.Item>
          <div className="voice-profile-form-grid">
            <Form.Item name="status" label="状态">
              <Select options={[{ value: "enabled", label: "启用" }, { value: "disabled", label: "停用" }]} />
            </Form.Item>
            <Form.Item name="is_default" label="默认音色">
              <Select options={[{ value: true, label: "设为默认" }, { value: false, label: "不设为默认" }]} />
            </Form.Item>
          </div>
        </Form>
      </Modal>
    </div>
  );
}
