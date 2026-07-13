import { Card, Empty, Pagination, Typography } from "antd";
import type { Asset, AssetListResponse } from "../../shared/types/asset";
import { AssetCard } from "./AssetCard";

type AssetGridProps = {
  assets: Asset[];
  result: AssetListResponse | null;
  loading: boolean;
  page: number;
  pageSize: number;
  productNameByID: Map<string, string>;
  onSelect: (asset: Asset) => void;
  onPageChange: (page: number, pageSize: number) => void;
};

export function AssetGrid({ assets, result, loading, page, pageSize, productNameByID, onSelect, onPageChange }: AssetGridProps) {
  const total = result?.total ?? 0;

  return (
    <Card
      className="asset-grid-card"
      title="素材列表"
      extra={<Typography.Text type="secondary">第 {result?.page ?? page} 页 / 共 {total} 条</Typography.Text>}
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
              onSelect={onSelect}
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
