import { Button, Empty, Modal, Tag, Tooltip, Typography, message } from "antd";
import { Check, ChevronDown, Pause, Play, Volume2 } from "lucide-react";
import { useState } from "react";
import type { MouseEvent } from "react";
import type { VoiceProfile } from "../../shared/types/voice";
import { useVoicePreview } from "./useVoicePreview";
import "./styles.css";

type VoiceProfilePickerProps = {
  profiles: VoiceProfile[];
  value: string;
  onChange: (profileID: string) => void;
};

export function VoiceProfilePicker({ profiles, value, onChange }: VoiceProfilePickerProps) {
  const [open, setOpen] = useState(false);
  const { playingProfileID, togglePreview } = useVoicePreview();
  const availableProfiles = profiles.filter((profile) => profile.status === "enabled" && profile.preview_status === "ready");
  const selectedProfile = availableProfiles.find((profile) => profile.id === value) ?? null;

  const previewProfile = (event: MouseEvent<HTMLElement>, profile: VoiceProfile) => {
    event.stopPropagation();
    void togglePreview(profile).catch((error) => message.error(error instanceof Error ? error.message : "样音播放失败"));
  };

  return (
    <>
      <div className="voice-profile-picker" data-testid="workbench-voice-selector">
        <button type="button" aria-label="选择音色" className="voice-profile-picker-trigger" onClick={() => setOpen(true)}>
          <span className="voice-profile-picker-icon"><Volume2 size={16} /></span>
          <span className="voice-profile-picker-copy">
            <strong>{selectedProfile?.name ?? "选择音色"}</strong>
            {selectedProfile ? <span>{selectedProfile.language} · {selectedProfile.style_tags.slice(0, 2).join(" / ") || "旁白"}</span> : null}
          </span>
          <ChevronDown size={15} />
        </button>
        {selectedProfile ? (
          <Tooltip title={playingProfileID === selectedProfile.id ? "停止试听" : "试听音色"}>
            <Button
              type="text"
              aria-label={playingProfileID === selectedProfile.id ? "停止试听" : "试听音色"}
              icon={playingProfileID === selectedProfile.id ? <Pause size={16} fill="currentColor" /> : <Play size={16} fill="currentColor" />}
              onClick={(event) => previewProfile(event, selectedProfile)}
            />
          </Tooltip>
        ) : null}
      </div>

      <Modal title="选择音色" open={open} onCancel={() => setOpen(false)} footer={null} width={720} destroyOnHidden>
        {availableProfiles.length === 0 ? (
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无可用音色" />
        ) : (
          <div className="voice-profile-option-grid" data-testid="voice-profile-modal">
            {availableProfiles.map((profile) => {
              const selected = profile.id === selectedProfile?.id;
              const playing = profile.id === playingProfileID;
              return (
                <article
                  key={profile.id}
                  className={`voice-profile-option${selected ? " is-selected" : ""}`}
                  data-testid={`voice-profile-option-${profile.id}`}
                  role="button"
                  aria-label={`选择音色 ${profile.name}`}
                  tabIndex={0}
                  onClick={() => {
                    onChange(profile.id);
                    setOpen(false);
                  }}
                  onKeyDown={(event) => {
                    if (event.key === "Enter" || event.key === " ") {
                      event.preventDefault();
                      onChange(profile.id);
                      setOpen(false);
                    }
                  }}
                >
                  <div className="voice-profile-option-heading">
                    <div>
                      <Typography.Text strong>{profile.name}</Typography.Text>
                      <Typography.Text type="secondary">{profile.language}</Typography.Text>
                    </div>
                    {selected ? <Check size={18} aria-label="已选择" /> : null}
                  </div>
                  <div className="voice-profile-option-tags">
                    {profile.style_tags.map((tag) => <Tag key={tag}>{tag}</Tag>)}
                    <Tag className="voice-profile-preview-kind">样音就绪</Tag>
                  </div>
                  <Typography.Paragraph className="voice-profile-option-sample">{profile.preview_text}</Typography.Paragraph>
                  <div className="voice-profile-option-actions">
                    <Tooltip title={playing ? "停止试听" : "试听音色"}>
                      <Button
                        type="text"
                        aria-label={playing ? `停止试听 ${profile.name}` : `试听 ${profile.name}`}
                        icon={playing ? <Pause size={17} fill="currentColor" /> : <Play size={17} fill="currentColor" />}
                        onClick={(event) => previewProfile(event, profile)}
                      />
                    </Tooltip>
                    {profile.is_default ? <Tag color="blue">默认</Tag> : null}
                  </div>
                </article>
              );
            })}
          </div>
        )}
      </Modal>
    </>
  );
}
