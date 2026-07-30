import { apiRequest } from "../../shared/api/server-api";
import type { Asset, AssetBulkArchiveResult, AssetEmbeddingListResponse, AssetEmbeddingRunResult, AssetFrameResponse, AssetListResponse, AssetReviewPayload, AssetSelectionResponse, AssetSemanticPreview, AssetSellingPointPayload, AssetSpeechSegment } from "../../shared/types/asset";
import type { Product, SellingPoint } from "../../shared/types/product";

export const listProducts = (path: string, token: string) => apiRequest<Product[]>(path, {}, token);
export const listSellingPoints = (path: string, token: string) => apiRequest<SellingPoint[]>(path, {}, token);
export const listAssets = (path: string, token: string) => apiRequest<AssetListResponse>(path, {}, token);
export const listAssetSelection = (path: string, token: string) => apiRequest<AssetSelectionResponse>(path, {}, token);
export const getAssetFrames = (assetID: string, token: string) => apiRequest<AssetFrameResponse>("/api/assets/" + assetID + "/frames", {}, token);
export const getSemanticPreview = (assetID: string, token: string) => apiRequest<AssetSemanticPreview>("/api/assets/" + assetID + "/semantic-preview", {}, token);
export const getAssetEmbeddings = (assetID: string, token: string) => apiRequest<AssetEmbeddingListResponse>("/api/assets/" + assetID + "/embeddings", {}, token);
export const getSpeechSegments = (assetID: string, token: string) => apiRequest<AssetSpeechSegment[]>("/api/assets/" + assetID + "/speech-segments", {}, token);
export const getAssetSellingPoints = (assetID: string, token: string) => apiRequest<SellingPoint[]>("/api/assets/" + assetID + "/selling-points", {}, token);
export const saveAssetReview = (assetID: string, values: AssetReviewPayload, token: string) => apiRequest<Asset>("/api/assets/" + assetID + "/review", { method: "PUT", body: JSON.stringify(values) }, token);
export const updateAssetArchiveState = (assetID: string, action: "archive" | "restore", token: string) => apiRequest<Asset>("/api/assets/" + assetID + "/" + action, { method: "POST" }, token);
export const archiveAssets = (assetIDs: string[], token: string) => apiRequest<AssetBulkArchiveResult>("/api/assets/bulk-archive", { method: "POST", body: JSON.stringify({ asset_ids: assetIDs }) }, token);
export const saveAssetSellingPoints = (assetID: string, values: AssetSellingPointPayload, token: string) => apiRequest<SellingPoint[]>("/api/assets/" + assetID + "/selling-points", { method: "PUT", body: JSON.stringify(values) }, token);
export const createAssetEmbeddings = (assetID: string, token: string) => apiRequest<AssetEmbeddingRunResult>("/api/assets/" + assetID + "/embeddings", { method: "POST" }, token);
