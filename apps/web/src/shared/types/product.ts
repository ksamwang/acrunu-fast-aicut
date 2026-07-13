export type Product = {
  id: string;
  name: string;
  description?: string;
  category?: string;
  status: string;
  metadata?: Record<string, unknown>;
};

export type SellingPoint = {
  id: string;
  product_id: string;
  title: string;
  description?: string;
  priority: number;
  status: string;
  asset_count?: number;
};

export type ProductStats = {
  product_id: string;
  asset_count: number;
  usable_asset_count: number;
  pending_analysis_count: number;
};
