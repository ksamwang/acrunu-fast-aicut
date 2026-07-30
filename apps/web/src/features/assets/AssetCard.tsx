import { Checkbox, Tag, Typography } from "antd";
import { assetDisplayTitle, assetFileDisplayName, assetVideoURL } from "../../shared/lib/asset-display";
import { formatDuration } from "../../shared/lib/format";
import { analysisStatusLabels, assetStatusLabels, cameraMovementLabels, shotSizeLabels, sourceTypeLabels, translateValue } from "../../shared/lib/labels";
import type { Asset } from "../../shared/types/asset";

type AssetCardProps = {
  asset: Asset;
  productName: string;
  selectionMode: boolean;
  selected: boolean;
  onSelect: (asset: Asset) => void;
  onToggleSelect: (asset: Asset) => void;
};

export function AssetCard({ asset, productName, selectionMode, selected, onSelect, onToggleSelect }: AssetCardProps) {
  const title = assetDisplayTitle(asset);
  const fileName = assetFileDisplayName(asset);
  const tags = [...(asset.scene_tags ?? []), ...(asset.subjects ?? [])].slice(0, 4);
  const archived = asset.status === "archived";

  return (
    <article
      data-testid={`asset-card-${asset.id}`}
      data-status={asset.status}
      className={`asset-library-card${archived ? " is-archived" : ""}${selectionMode ? " is-selection-mode" : ""}${selected ? " is-selected" : ""}`}
    >
      <button
        type="button"
        className="asset-library-card-main"
        onClick={() => selectionMode ? (!archived && onToggleSelect(asset)) : onSelect(asset)}
        aria-label={selectionMode ? `${selected ? "取消选择" : "选择"} ${title}` : title}
        disabled={selectionMode && archived}
      >
        <div className="asset-card-media">
          <video src={assetVideoURL(asset)} muted preload="metadata" />
          <div className="asset-card-scrim" />
          <Tag className="asset-card-status">{translateValue(asset.status, assetStatusLabels)}</Tag>
          <Tag className="asset-card-type">{translateValue(asset.source_type, sourceTypeLabels)}</Tag>
          <span className="asset-card-duration">{formatDuration(asset.duration_ms)}</span>
        </div>
        <div className="asset-card-body">
          <Typography.Text className="asset-card-title">{title}</Typography.Text>
          <Typography.Text className="asset-card-file">{fileName}</Typography.Text>
          <div className="asset-card-meta">
            <span>{productName}</span>
            <span>{asset.width && asset.height ? `${asset.width}x${asset.height}` : "未知分辨率"}</span>
          </div>
          <div className="asset-card-labels">
            {asset.shot_size ? <Tag>{translateValue(asset.shot_size, shotSizeLabels)}</Tag> : null}
            {asset.camera_movement ? <Tag>{translateValue(asset.camera_movement, cameraMovementLabels)}</Tag> : null}
            {asset.analysis_status ? <Tag color="blue">{translateValue(asset.analysis_status, analysisStatusLabels)}</Tag> : null}
          </div>
          <Typography.Text className="asset-card-description">{asset.scene_description || "暂无画面描述"}</Typography.Text>
          <div className="asset-card-tags">
            {tags.length > 0 ? tags.map((tag) => <span key={tag}>{tag}</span>) : <span>暂无标签</span>}
          </div>
        </div>
      </button>
      {selectionMode ? (
        <Checkbox
          className="asset-card-checkbox"
          checked={selected}
          disabled={archived}
          aria-label={archived ? `${title} 已归档` : `${selected ? "取消选择" : "选择"} ${title}`}
          onChange={() => onToggleSelect(asset)}
        />
      ) : null}
    </article>
  );
}
