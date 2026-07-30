import { Button, Card, Empty, Pagination, Space, Typography } from "antd";
import { Archive, CheckSquare2, ScanSearch, X } from "lucide-react";
import type { Asset, AssetListResponse } from "../../shared/types/asset";
import { AssetCard } from "./AssetCard";

type AssetGridProps = {
  assets: Asset[];
  result: AssetListResponse | null;
  loading: boolean;
  page: number;
  pageSize: number;
  semanticQuery: string;
  productNameByID: Map<string, string>;
  selectionMode: boolean;
  selectedAssetIDs: Set<string>;
  selectingAll: boolean;
  archivingSelected: boolean;
  reanalyzingSelected: boolean;
  allResultsSelected: boolean;
  onSelect: (asset: Asset) => void;
  onEnterSelectionMode: () => void;
  onExitSelectionMode: () => void;
  onToggleSelection: (asset: Asset) => void;
  onTogglePageSelection: (assetIDs: string[]) => void;
  onToggleAllResults: () => void;
  onArchiveSelected: () => void;
  onReanalyzeSelected: () => void;
  onPageChange: (page: number, pageSize: number) => void;
};

export function AssetGrid({
  assets,
  result,
  loading,
  page,
  pageSize,
  semanticQuery,
  productNameByID,
  selectionMode,
  selectedAssetIDs,
  selectingAll,
  archivingSelected,
  reanalyzingSelected,
  allResultsSelected,
  onSelect,
  onEnterSelectionMode,
  onExitSelectionMode,
  onToggleSelection,
  onTogglePageSelection,
  onToggleAllResults,
  onArchiveSelected,
  onReanalyzeSelected,
  onPageChange
}: AssetGridProps) {
  const total = result?.total ?? 0;
  const pageSelectableIDs = assets.filter((asset) => asset.status !== "archived").map((asset) => asset.id);
  const allPageSelected = pageSelectableIDs.length > 0 && pageSelectableIDs.every((assetID) => selectedAssetIDs.has(assetID));

  return (
    <Card
      className="asset-grid-card"
      title="素材列表"
      extra={selectionMode ? (
        <Space className="asset-selection-toolbar" wrap size={6}>
          <Typography.Text strong>已选 {selectedAssetIDs.size} 项</Typography.Text>
          <Button size="small" disabled={pageSelectableIDs.length === 0 || archivingSelected || reanalyzingSelected} onClick={() => onTogglePageSelection(pageSelectableIDs)}>
            {allPageSelected ? "取消本页" : "全选本页"}
          </Button>
          <Button size="small" loading={selectingAll} disabled={archivingSelected || reanalyzingSelected} onClick={onToggleAllResults}>
            {allResultsSelected ? "取消全选" : "全选当前结果"}
          </Button>
          <Button size="small" type="primary" icon={<ScanSearch size={14} />} loading={reanalyzingSelected} disabled={selectedAssetIDs.size === 0 || archivingSelected} onClick={onReanalyzeSelected}>
            批量 VLM
          </Button>
          <Button size="small" type="primary" danger icon={<Archive size={14} />} loading={archivingSelected} disabled={selectedAssetIDs.size === 0 || reanalyzingSelected} onClick={onArchiveSelected}>
            归档所选
          </Button>
          <Button size="small" type="text" icon={<X size={15} />} aria-label="退出素材选择" disabled={archivingSelected || reanalyzingSelected} onClick={onExitSelectionMode} />
        </Space>
      ) : (
        <Space size={10}>
          <Typography.Text type="secondary">{semanticQuery ? "语义结果" : `第 ${result?.page ?? page} 页`} / 共 {total} 条</Typography.Text>
          <Button size="small" icon={<CheckSquare2 size={15} />} onClick={onEnterSelectionMode}>批量选择</Button>
        </Space>
      )}
      loading={loading}
    >
      {assets.length === 0 ? (
        <Empty description="暂无匹配素材" />
      ) : (
        <div className="asset-card-grid">
          {assets.map((asset) => (
            <AssetCard
              key={asset.id}
              asset={asset}
              productName={productNameByID.get(asset.product_id) ?? asset.product_id ?? "-"}
              selectionMode={selectionMode}
              selected={selectedAssetIDs.has(asset.id)}
              onSelect={onSelect}
              onToggleSelect={onToggleSelection}
            />
          ))}
        </div>
      )}
      <div className="asset-pagination">
        <Pagination
          current={result?.page ?? page}
          pageSize={result?.page_size ?? pageSize}
          total={total}
          showSizeChanger
          pageSizeOptions={["10", "20", "50"]}
          onChange={onPageChange}
        />
      </div>
    </Card>
  );
}
