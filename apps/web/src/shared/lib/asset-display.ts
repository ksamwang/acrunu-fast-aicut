import type { Asset } from "../types/asset";

export function assetVideoURL(asset: Asset) {
  return `/storage/${encodeURI(asset.storage_key)}`;
}

export function assetFileDisplayName(asset: Asset) {
  if (asset.source_original_name && /^clean-shot\.[^.]+$/i.test(asset.file_name)) {
    return asset.source_original_name;
  }
  return asset.file_name || asset.source_original_name || "-";
}

export function assetDisplayTitle(asset: Asset) {
  return asset.asset_name || assetFileDisplayName(asset);
}
